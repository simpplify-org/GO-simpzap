package app

import (
	"context"
	"errors"
	"fmt"
	"go.mau.fi/whatsmeow"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/simpplify-org/GO-simpzap/pkg/whatsapp"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"google.golang.org/protobuf/proto"
)

type WhatsAppService struct {
	Repo                     *DeviceRepository
	MessageHistoryRepository *MessageHistoryRepository
	ListContactsRepository   *ContactListRepository
	ZapPkg                   *whatsapp.ZapPkg
}

func NewWhatsAppService(repo *DeviceRepository, messageHist *MessageHistoryRepository, listContacts *ContactListRepository) *WhatsAppService {
	return &WhatsAppService{Repo: repo, MessageHistoryRepository: messageHist, ListContactsRepository: listContacts, ZapPkg: whatsapp.NewZapPkg()}
}

func (s *WhatsAppService) saveMessageStatus(ctx context.Context, deviceID, number, message, status string) error {
	device, err := s.Repo.GetByID(ctx, deviceID)
	if err != nil {
		return err
	}

	_, saveErr := s.MessageHistoryRepository.InsertHistory(ctx, &MessageHistory{
		DeviceName: device.Name,
		TenantID:   device.TenantID,
		DeviceID:   deviceID,
		Number:     number,
		Message:    message,
		Status:     status,
	})
	return saveErr
}

func (s *WhatsAppService) SendMessage(deviceID, number, message string) (string, error) {
	ctx := context.Background()
	client, _, err := s.ZapPkg.StartClient(ctx, deviceID)
	if err != nil {
		return "device inválido", fmt.Errorf("erro ao iniciar client: %w", err)
	}
	defer func() {
		if cerr := whatsapp.CloseClient(client); cerr != nil {
			log.Printf("⚠️ erro ao fechar client WhatsApp: %v", cerr)
		}
	}()

	if client.Store.ID == nil || !client.IsConnected() {
		status := "device expirado, abra uma nova sessão"
		_ = s.saveMessageStatus(ctx, deviceID, number, message, status)
		return status, fmt.Errorf(status)
	}

	jid := types.NewJID(number, "s.whatsapp.net")

	resp, err := client.SendMessage(ctx, jid, &waProto.Message{
		Conversation: proto.String(message),
	})

	status := "sent"
	if err != nil {
		errMsg := err.Error()

		switch {
		case strings.Contains(errMsg, "websocket disconnected"),
			strings.Contains(errMsg, "failed to get device list"),
			strings.Contains(errMsg, "failed to send usync query"):
			status = "device expirado, abra uma nova sessão"
			err = fmt.Errorf(status)

		default:
			status = "failed"
		}
		log.Printf("Erro ao enviar mensagem: %v", err)
	} else {
		log.Printf("Mensagem enviada com ID: %s", resp.ID)
	}

	saveErr := s.saveMessageStatus(ctx, deviceID, number, message, status)
	if saveErr != nil {
		log.Printf("Erro ao salvar histórico da mensagem: %v", saveErr)
	}

	return status, err
}

type SendTask struct {
	Number string
	Text   string
}

func (s *WhatsAppService) CreateOrStartClientService(deviceID string) (client *whatsmeow.Client, qr <-chan whatsmeow.QRChannelItem, err error) {
	//chamar o defer na funcao que chamar esta aqui
	ctx := context.Background()
	client, _, err = s.ZapPkg.StartClient(ctx, deviceID)
	if err != nil {
		return client, nil, fmt.Errorf("erro ao iniciar client: %w", err)
	}
	return client, nil, nil
}

func (s *WhatsAppService) SendManyMessagesE2E(deviceID string, numbers []string, text string) error {
	ctx := context.Background()
	client, _, err := s.ZapPkg.StartClient(ctx, deviceID)
	if err != nil {
		return fmt.Errorf("erro ao iniciar client: %w", err)
	}
	defer func() {
		if cerr := whatsapp.CloseClient(client); cerr != nil {
			log.Printf("[Service] erro ao fechar client WhatsApp: %v", cerr)
		}
	}()

	if client.Store.ID == nil || !client.IsConnected() {
		status := "device expirado, abra uma nova sessão"
		for _, n := range numbers {
			_ = s.saveMessageStatus(ctx, deviceID, n, text, status)
		}
		return fmt.Errorf(status)
	}

	const maxWorkers = 5
	const delayBetweenBatches = 1 * time.Second

	tasks := make(chan SendTask, len(numbers))
	results := make(chan error, len(numbers))
	var wg sync.WaitGroup

	for i := 0; i < maxWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for task := range tasks {
				err = whatsapp.SendMessage(ctx, client, task.Number, task.Text, false)
				status := "sent"
				if err != nil {
					errMsg := err.Error()
					if strings.Contains(errMsg, "no signal session established") {
						log.Printf("Worker %d: Erro de sessão Signal para %s. Tentando novamente em 3s...", workerID, task.Number)
						time.Sleep(3 * time.Second)
					}

					switch {
					case strings.Contains(errMsg, "websocket disconnected"),
						strings.Contains(errMsg, "failed to get device list"),
						strings.Contains(errMsg, "failed to send usync query"):
						status = "device expirado, abra uma nova sessão"
						err = fmt.Errorf(status)
					default:
						status = "failed"
					}
					results <- fmt.Errorf("erro ao enviar para %s: %w", task.Number, err)
				} else {
					log.Printf("Worker %d: Mensagem enviada para %s", workerID, task.Number)
					results <- nil // Sucesso
				}

				if saveErr := s.saveMessageStatus(ctx, deviceID, task.Number, task.Text, status); saveErr != nil {
					log.Printf("[Service] Erro ao salvar status para %s: %v", task.Number, saveErr)
				}
				time.Sleep(500 * time.Millisecond)
			}
		}(i)
	}

	for i, number := range numbers {
		tasks <- SendTask{Number: number, Text: text}
		if (i+1)%maxWorkers == 0 {
			time.Sleep(delayBetweenBatches)
		}
	}
	close(tasks)

	wg.Wait()
	close(results)

	var errMessage []error
	successCount := 0
	totalCount := len(numbers)

	for res := range results {
		if res != nil {
			errMessage = append(errMessage, res)
		} else {
			successCount++
		}
	}

	if len(errMessage) > 0 {
		if successCount == 0 {
			return fmt.Errorf("todas as mensagens falharam: %v", errMessage[0])
		}
		log.Printf("[Service] Algumas mensagens falharam (%d de %d)", len(errMessage), totalCount)
		for _, e := range errMessage {
			log.Println("→", e)
		}
		return nil
	}
	log.Printf("[Service] Envio em massa concluído com sucesso: %d mensagens enviadas", successCount)
	return nil
}

func (s *WhatsAppService) SaveConnectedDevice(ctx context.Context, name, tenantID, number, sessionPath string) (*Device, error) {
	existingDevice, err := s.FindDeviceByTenantAndNumber(ctx, tenantID, number)

	sessionBytes, err := os.ReadFile(sessionPath)
	if err != nil {
		return nil, fmt.Errorf("erro ao ler sessão: %w", err)
	}

	if existingDevice != nil {
		filter := bson.M{"_id": existingDevice.ID}
		update := bson.M{
			"$set": bson.M{
				"session_db": sessionBytes,
				"updated_at": time.Now().Unix(),
				"connected":  true,
			},
		}
		_, err = s.Repo.Collection.UpdateOne(ctx, filter, update)
		if err != nil {
			return nil, fmt.Errorf("erro ao atualizar no MongoDB: %w", err)
		}
		existingDevice.SessionDB = sessionBytes
		return existingDevice, nil
	} else {
		device := &Device{
			Name:      name,
			TenantID:  tenantID,
			Number:    number,
			CreatedAt: time.Now().Unix(),
			UpdatedAt: time.Now().Unix(),
			SessionDB: sessionBytes,
			Connected: true,
		}

		res, err := s.Repo.Collection.InsertOne(ctx, device)
		if err != nil {
			return nil, fmt.Errorf("erro ao salvar no MongoDB: %w", err)
		}

		device.ID = res.InsertedID.(primitive.ObjectID)
		return device, nil
	}
}

func (s *WhatsAppService) FindDeviceByTenantAndNumber(ctx context.Context, tenantID, number string) (*Device, error) {
	var device Device
	err := s.Repo.Collection.FindOne(ctx, bson.M{
		"tenant_id": tenantID,
		"number":    number,
	}).Decode(&device)

	if err != nil {
		return nil, err
	}
	return &device, nil
}

func (s *WhatsAppService) GetAllDevices(ctx context.Context, tenantID string) ([]*Device, error) {
	if tenantID == "" {
		return nil, fmt.Errorf("tenantID não pode ser vazio")
	}
	filter := bson.M{"tenant_id": tenantID}

	cursor, err := s.Repo.Collection.Find(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar dispositivos: %w", err)
	}
	var devices []*Device
	if err = cursor.All(ctx, &devices); err != nil {
		return nil, fmt.Errorf("erro ao decodificar dispositivos: %w", err)
	}
	fmt.Printf("Dispositivos encontrados para tenant %s: %d\n", tenantID, len(devices))
	return devices, nil
}

func (s *WhatsAppService) InsertListContact(ctx context.Context, data ContactListRequest) (*mongo.InsertOneResult, error) {
	return s.ListContactsRepository.InsertListContact(ctx, data)
}

func (s *WhatsAppService) ListContacts(ctx context.Context, tenantId string) ([]ContactListResponse, error) {
	if tenantId == "" {
		return nil, errors.New("tenant id não pode ser vazio")
	}

	contacts, err := s.ListContactsRepository.ListContacts(ctx, tenantId)
	if err != nil {
		return []ContactListResponse{}, fmt.Errorf("erro ao ler lista: %w", err)
	}

	var response []ContactListResponse
	for _, contact := range contacts {
		response = append(response, ContactListResponse{
			ID:        contact.ID,
			TenantID:  contact.TenantID,
			DeviceID:  contact.DeviceID,
			Name:      contact.Name,
			Number:    contact.Number,
			CreatedAt: contact.CreatedAt,
		})
	}

	return response, nil
}

func (s *WhatsAppService) DeleteContact(ctx context.Context, id string) error {
	return s.ListContactsRepository.DeleteContact(ctx, id)
}
