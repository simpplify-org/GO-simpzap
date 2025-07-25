package app

import (
	"context"
	"fmt"
	"github.com/simpplify-org/GO-simpzap/pkg/whatsapp"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
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
	sessionBytes, err := os.ReadFile(sessionPath)
	if err != nil {
		return nil, fmt.Errorf("erro ao ler sessão: %w", err)
	}

	device := &Device{
		TenantID:  tenantID,
		Number:    number,
		CreatedAt: time.Now().Unix(),
		SessionDB: sessionBytes,
	}

	res, err := s.Repo.Collection.InsertOne(ctx, device)
	if err != nil {
		return nil, fmt.Errorf("erro ao salvar no MongoDB: %w", err)
	}

	device.ID = res.InsertedID.(primitive.ObjectID)
	return device, nil
}
