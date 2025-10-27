package whatsapp

import (
	"context"
	"fmt"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/mdp/qrterminal/v3"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
	"log"
	"os"
)

var (
	dbLog     = waLog.Stdout("Database", "INFO", true)
	clientLog = waLog.Stdout("Client", "INFO", true)
)

type ZapPkg struct {
	PostgresDSN string
}

func NewZapPkg() *ZapPkg {
	if os.Getenv("POSTGRES_DSN") == "" {
		if err := godotenv.Load(); err != nil {
			log.Println("[ZapPkg] Nenhum arquivo .env encontrado, usando variáveis de ambiente ou padrão.")
		}
	}
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		log.Println("[ZapPkg] POSTGRES_DSN não encontrado, usando DSN padrão.")
	} else {
		log.Printf("[ZapPkg] DSN carregado do .env: %s\n", dsn)
	}
	return &ZapPkg{
		PostgresDSN: dsn,
	}
}
func eventHandler(evt interface{}) {
	switch v := evt.(type) {
	case *events.Message:
		fmt.Println("Received message:", v.Message.GetConversation())
	}
}

// createClientContainer configura o sqlstore com a conexão PostgreSQL.
func (z *ZapPkg) createClientContainer(ctx context.Context) (*sqlstore.Container, error) {
	container, err := sqlstore.New(ctx, "postgres", z.PostgresDSN, dbLog)
	if err != nil {
		return nil, fmt.Errorf("erro ao criar sqlstore com PostgreSQL: %w", err)
	}
	return container, nil
}

func (z *ZapPkg) CreateClient(ctx context.Context, deviceID string) (*whatsmeow.Client, <-chan whatsmeow.QRChannelItem, error) {
	container, err := z.createClientContainer(ctx)
	if err != nil {
		return nil, nil, err
	}

	//TODO passar o JID salvo no banco
	jid, err := types.ParseJID(deviceID + types.DefaultUserServer)
	if err != nil {
		return nil, nil, fmt.Errorf("JID inválido: %w", err)
	}

	deviceStore, err := container.GetDevice(ctx, jid)
	if err != nil {
		return nil, nil, fmt.Errorf("erro ao obter device: %w", err)
	}
	if deviceStore == nil {
		deviceStore, err = container.GetFirstDevice(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("erro ao criar device: %w", err)
		}
	}

	client := whatsmeow.NewClient(deviceStore, clientLog)
	client.AddEventHandler(eventHandler)

	var qrChan <-chan whatsmeow.QRChannelItem
	if client.Store.ID == nil {
		qrChan, _ = client.GetQRChannel(ctx)
	}
	if err := client.Connect(); err != nil {
		return nil, nil, fmt.Errorf("erro ao conectar no WhatsApp: %w", err)
	}
	return client, qrChan, nil
}

func (z *ZapPkg) StartClient(ctx context.Context, deviceID string) (client *whatsmeow.Client, qrcode <-chan whatsmeow.QRChannelItem, err error) {
	client, qrcode, err = z.CreateClient(ctx, deviceID)
	if err != nil {
		return nil, nil, err
	}

	if qrcode != nil {
		go func() {
			for evt := range qrcode {
				if evt.Event == "code" {
					fmt.Println("Scan this QR code to log in:")
					qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
				}
			}
		}()
	}

	if client.IsConnected() {
		log.Println("Successfully connected to database and CLIENT")
		// FetchAppState e PutCachedSessions (mantidos do original)
		if err := client.FetchAppState(ctx, appstate.WAPatchCriticalUnblockLow, true, false); err != nil {
			log.Printf("⚠️ Erro ao sincronizar estado do app WAPatchCriticalUnblockLow: %v", err)
		} else {
			log.Println("✅ Estado do app sincronizado com sucesso. WAPatchCriticalUnblockLow")
		}
	}

	log.Println("Cliente WhatsApp conectado com sucesso!")
	return client, qrcode, nil
}

// CloseClient desconecta o cliente. O parâmetro tmpFile foi removido.
func CloseClient(client *whatsmeow.Client) error {
	if client != nil {
		client.Disconnect()
	}
	return nil
}

// SendMessage (mantida do original)
func SendMessage(ctx context.Context, client *whatsmeow.Client, number string, text string, forward bool) error {
	jid := types.NewJID(number, types.DefaultUserServer)
	msg := &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text: proto.String(text),
			ContextInfo: &waE2E.ContextInfo{
				IsForwarded: proto.Bool(forward),
			},
		},
	}

	_, err := client.SendMessage(ctx, jid, msg)
	if err != nil {
		log.Printf("Erro ao enviar para %s: %v", number, err)
	}
	return err
}
