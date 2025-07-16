package app

import (
	"context"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
	"github.com/mdp/qrterminal/v3"
	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
	"os"
)

func InitWhatsAppClient() (*whatsmeow.Client, <-chan whatsmeow.QRChannelItem, error) {
	ctx := context.Background()
	dbLog := waLog.Stdout("Database", "DEBUG", true)

	container, err := sqlstore.New(ctx, "sqlite3", "file:.data/session.db?_foreign_keys=on", dbLog)
	if err != nil {
		return nil, nil, fmt.Errorf("Erro ao abrir o DB: %w", err)
	}

	deviceStore, err := container.GetFirstDevice(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("Erro ao pegar o device: %w", err)
	}

	clientLog := waLog.Stdout("Client", "DEBUG", true)
	client := whatsmeow.NewClient(deviceStore, clientLog)

	var qrChan <-chan whatsmeow.QRChannelItem
	if client.Store.ID == nil {
		qrChan, _ = client.GetQRChannel(ctx)
	}

	err = client.Connect()
	if err != nil {
		return nil, nil, fmt.Errorf("Erro ao conectar client: %w", err)
	}

	return client, qrChan, nil
}

func SendMessage(client *whatsmeow.Client, to string, text string) error {
	jid := types.NewJID(to, "s.whatsapp.net")
	msg := &waProto.Message{
		Conversation: proto.String(text),
	}

	_, err := client.SendMessage(context.Background(), jid, msg, whatsmeow.SendRequestExtra{})
	return err
}

func PrintCompactQR(data string) {
	config := qrterminal.Config{
		Level:      qrterminal.L,
		Writer:     os.Stdout,
		QuietZone:  1,
		HalfBlocks: true,
	}
	qrterminal.GenerateWithConfig(data, config)
}
