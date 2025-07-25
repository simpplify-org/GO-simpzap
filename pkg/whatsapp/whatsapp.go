package whatsapp

import (
	"context"
	"fmt"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
	"log"
	"os"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/mdp/qrterminal/v3"
	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"
)

var (
	dbLog     = waLog.Stdout("Database", "INFO", true)
	clientLog = waLog.Stdout("Client", "INFO", true)
)

func CreateClient(deviceID string) (*whatsmeow.Client, <-chan whatsmeow.QRChannelItem, string, error) {
	dbLog := waLog.Stdout("Database", "INFO", true)
	clientLog := waLog.Stdout("Client", "INFO", true)

	sessionPath := fmt.Sprintf(".data/session-%s.db", deviceID)

	container, err := sqlstore.New(context.Background(), "sqlite3", "file:"+sessionPath+"?_foreign_keys=on", dbLog)
	if err != nil {
		return nil, nil, "", fmt.Errorf("erro ao criar sqlstore: %w", err)
	}

	deviceStore, err := container.GetFirstDevice(context.Background())
	if err != nil {
		return nil, nil, "", fmt.Errorf("erro ao obter device: %w", err)
	}

	client := whatsmeow.NewClient(deviceStore, clientLog)

	var qrChan <-chan whatsmeow.QRChannelItem
	if client.Store.ID == nil {
		qrChan, _ = client.GetQRChannel(context.Background())
	}

	if err := client.Connect(); err != nil {
		return nil, nil, "", fmt.Errorf("erro ao conectar no WhatsApp: %w", err)
	}

	time.Sleep(500 * time.Millisecond)
	return client, qrChan, sessionPath, nil
}

// 2. Inicia client a partir do conteúdo binário da session (arquivo .db)
// grava arquivo temporário, cria client, conecta e apaga arquivo depois
func StartClient(sessionData []byte) (*whatsmeow.Client, <-chan whatsmeow.QRChannelItem, error) {
	tmpFile := fmt.Sprintf(".data/session-temp-%d.db", time.Now().UnixNano())

	if err := os.WriteFile(tmpFile, sessionData, 0600); err != nil {
		return nil, nil, fmt.Errorf("erro ao gravar arquivo temporário: %w", err)
	}

	container, err := sqlstore.New(context.Background(), "sqlite3", "file:"+tmpFile+"?_foreign_keys=on", dbLog)
	if err != nil {
		os.Remove(tmpFile)
		return nil, nil, fmt.Errorf("erro ao criar sqlstore: %w", err)
	}

	deviceStore, err := container.GetFirstDevice(context.Background())
	if err != nil {
		os.Remove(tmpFile)
		return nil, nil, fmt.Errorf("erro ao obter device: %w", err)
	}

	client := whatsmeow.NewClient(deviceStore, clientLog)

	var qrChan <-chan whatsmeow.QRChannelItem
	if client.Store.ID == nil {
		qrChan, _ = client.GetQRChannel(context.Background())

		go func() {
			for evt := range qrChan {
				if evt.Event == "code" {
					fmt.Println("Scan this QR code to log in:")
					qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
				}
			}
		}()
	}

	err = client.Connect()
	if err != nil {
		os.Remove(tmpFile)
		return nil, nil, fmt.Errorf("erro ao conectar no WhatsApp: %w", err)
	} else {
		log.Println("Successfully connected to database and CLIENT")
	}
	return client, qrChan, nil
}

func CloseClient(client *whatsmeow.Client) error {
	if client == nil {
		return nil
	}
	client.Disconnect()
	return nil
}

func SendMessage(client *whatsmeow.Client, to string, text string) error {
	jid := types.NewJID(to, "s.whatsapp.net")
	msg := &waProto.Message{
		Conversation: proto.String(text),
	}
	_, err := client.SendMessage(context.Background(), jid, msg, whatsmeow.SendRequestExtra{})
	return err
}
