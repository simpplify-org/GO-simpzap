package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/simpplify-org/GO-simpzap/pkg/whatsapp"
	"go.mau.fi/whatsmeow"
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
}

func NewWhatsAppService(repo *DeviceRepository, messageHist *MessageHistoryRepository, listContacts *ContactListRepository) *WhatsAppService {
	return &WhatsAppService{Repo: repo, MessageHistoryRepository: messageHist, ListContactsRepository: listContacts}
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

func (s *WhatsAppService) SendMessage(client *whatsmeow.Client, deviceID, number, message string) (string, error) {
	ctx := context.Background()

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

func (s *WhatsAppService) SendMessageAsync(deviceID, number, message string) error {
	ctx := context.Background()

	sessionBytes, err := s.Repo.GetSessionByDeviceID(ctx, deviceID)
	if err != nil {
		return fmt.Errorf("não foi possível obter sessão: %w", err)
	}

	client, qrChan, err := whatsapp.StartClient(sessionBytes)
	if err != nil {
		return fmt.Errorf("erro ao iniciar client: %w", err)
	}
	//defer whatsapp.CloseClient(client)

	go func() {
		for evt := range qrChan {
			if evt.Event == "code" {
				fmt.Println("QR Code para reautenticação:", evt.Code)
			}
		}
	}()

	err = whatsapp.SendMessage(client, number, message)

	status := "sent"
	if err != nil {
		status = "failed"
	}

	_, saveErr := s.MessageHistoryRepository.InsertHistory(ctx, &MessageHistory{
		ID:       primitive.NewObjectID(),
		TenantID: "",
		DeviceID: deviceID,
		Number:   number,
		Message:  message,
		Status:   status,
	})
	if saveErr != nil {
		log.Printf("Erro ao iniciar client: %v", saveErr)
	}

	if err != nil {
		return fmt.Errorf("erro ao enviar mensagem: %w", err)
	}

	return nil
}

func (s *WhatsAppService) SaveConnectedDevice(ctx context.Context, name, tenantID, number, sessionPath string) (*Device, error) {
	// Tenta encontrar dispositivo existente
	existingDevice, err := s.FindDeviceByTenantAndNumber(ctx, tenantID, number)

	// Lê os bytes da sessão atual
	sessionBytes, err := os.ReadFile(sessionPath)
	if err != nil {
		return nil, fmt.Errorf("erro ao ler sessão: %w", err)
	}

	if existingDevice != nil {
		// Atualiza dispositivo existente
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
		// Cria novo dispositivo
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

func (s *WhatsAppService) SendManyMessages(deviceId string, numbers []string, message string) error {

	sessionBytes, err := s.Repo.GetSessionByDeviceID(context.Background(), deviceId)
	if err != nil {
		return fmt.Errorf("não foi possível obter sessão: %w", err)
	}

	client, _, err := whatsapp.StartClient(sessionBytes)
	if err != nil {
		return fmt.Errorf("erro ao iniciar client: %w", err)
	}

	var wg sync.WaitGroup
	errChan := make(chan error, len(numbers))
	var successCount int
	var mu sync.Mutex

	for _, number := range numbers {
		wg.Add(1)
		go func(num string) {
			defer wg.Done()

			if _, err := s.SendMessage(client, deviceId, num, message); err != nil {
				errChan <- fmt.Errorf("erro ao enviar para %s: %w", num, err)
			} else {
				mu.Lock()
				successCount++
				mu.Unlock()
				fmt.Println("mensagem para ", num)
			}
		}(number)
	}

	wg.Wait()
	close(errChan)

	var errors []error
	for err := range errChan {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		log.Printf("Envio em massa: %d sucessos, %d erros", successCount, len(errors))
		for _, err := range errors {
			log.Printf("Erro: %v", err)
		}

		if successCount == 0 {
			return fmt.Errorf("todas as mensagens falharam: %v", errors[0])
		}

		log.Printf("Algumas mensagens falharam, mas %d foram enviadas com sucesso", successCount)
		return nil
	}

	log.Printf("Envio em massa concluído: %d mensagens enviadas com sucesso", successCount)
	return nil
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
