package app

import (
	"context"
	"fmt"
	"github.com/simpplify-org/GO-simpzap/pkg/whatsapp"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"google.golang.org/protobuf/proto"
	"os"
	"time"
)

type WhatsAppService struct {
	Repo *DeviceRepository
}

func NewWhatsAppService(repo *DeviceRepository) *WhatsAppService {
	return &WhatsAppService{Repo: repo}
}

func (s *WhatsAppService) SendMessage(deviceID, number, message string) error {
	sessionBytes, err := s.Repo.GetSessionByDeviceID(context.Background(), deviceID)
	if err != nil {
		return fmt.Errorf("não foi possível obter sessão: %w", err)
	}

	client, _, err := whatsapp.StartClient(sessionBytes)
	if err != nil {
		return fmt.Errorf("erro ao iniciar client: %w", err)
	}
	defer client.Disconnect()

	jid := types.NewJID(number, "s.whatsapp.net")
	ctx := context.Background()
	_, err = client.SendMessage(ctx, jid, &waProto.Message{
		Conversation: proto.String(message),
	})
	if err != nil {
		return fmt.Errorf("erro ao enviar mensagem: %w", err)
	}
	return nil
}

func (s *WhatsAppService) SendMessageAsync(deviceID, number, message string) error {
	sessionBytes, err := s.Repo.GetSessionByDeviceID(context.Background(), deviceID)
	if err != nil {
		return fmt.Errorf("não foi possível obter sessão: %w", err)
	}

	client, qrChan, err := whatsapp.StartClient(sessionBytes)
	if err != nil {
		return fmt.Errorf("erro ao iniciar client: %w", err)
	}
	defer whatsapp.CloseClient(client)

	go func() {
		for evt := range qrChan {
			if evt.Event == "code" {
				fmt.Println("QR Code para reautenticação:", evt.Code)
			}
		}
	}()

	err = whatsapp.SendMessage(client, number, message)
	if err != nil {
		return fmt.Errorf("erro ao enviar mensagem: %w", err)
	}

	return nil
}

func (s *WhatsAppService) SaveConnectedDevice(ctx context.Context, tenantID, number, sessionPath string) (*Device, error) {
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
