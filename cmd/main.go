package main

import (
	"context"
	"fmt"
	"github.com/gorilla/websocket"
	_ "github.com/mattn/go-sqlite3"
	"github.com/mdp/qrterminal/v3"
	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
	"log"
	"net/http"
	"os"
)

const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func printCompactQR(data string) {
	config := qrterminal.Config{
		Level:      qrterminal.L,
		Writer:     os.Stdout,
		QuietZone:  1,
		HalfBlocks: true,
	}

	qrterminal.GenerateWithConfig(data, config)
}

func sendMessage(client *whatsmeow.Client, to string, text string) error {
	jid := types.NewJID(to, "s.whatsapp.net")

	msg := &waProto.Message{
		Conversation: proto.String(text),
	}

	_, err := client.SendMessage(
		context.Background(),
		jid,
		msg,
		whatsmeow.SendRequestExtra{},
	)
	return err
}

func wsHandler(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}
	fmt.Println(colorGreen + "WebSocket Connected" + colorReset)
	defer ws.Close()

	dbLog := waLog.Stdout("Database", "DEBUG", true)
	ctx := context.Background()
	container, err := sqlstore.New(ctx, "sqlite3", "file:examplestore.db?_foreign_keys=on", dbLog)
	if err != nil {
		ws.WriteJSON(map[string]string{
			"event": "error",
			"msg":   "Erro ao abrir DB",
		})
	}

	deviceStore, err := container.GetFirstDevice(ctx)
	if err != nil {
		ws.WriteJSON(map[string]string{
			"event": "error",
			"msg":   "Erro ao pegar o device",
		})
	}

	clientLog := waLog.Stdout("Client", "DEBUG", true)
	client := whatsmeow.NewClient(deviceStore, clientLog)
	client.AddEventHandler(func(evt interface{}) {
		switch v := evt.(type) {
		case *events.Disconnected:
			ws.WriteJSON(map[string]string{
				"event": "disconnected",
				"msg":   "Desconectado. Escaneie o QR Code novamente.",
			})
		case *events.Connected:
			ws.WriteJSON(map[string]string{
				"event": "connected",
				"msg":   "Conectado com sucesso!",
			})
		default:
			_ = v
		}

	})

	type IncomingMessage struct {
		Event string `json:"event"`
		To    string `json:"to"`
		Text  string `json:"text"`
	}

	go func() {
		for {
			var msg IncomingMessage
			err := ws.ReadJSON(&msg)
			if err != nil {
				log.Println("Erro ao ler mensagem do cliente: ", err)
				break
			}

			if msg.Event == "send_message" {
				err := sendMessage(client, msg.To, msg.Text)
				if err != nil {
					ws.WriteJSON(map[string]string{
						"event": "send_error",
						"msg":   "Erro ao enviar mensagem: " + err.Error(),
					})
				} else {
					ws.WriteJSON(map[string]string{
						"event": "message_sent",
						"msg":   "Mensagem enviada para " + msg.To,
					})
				}
			}
		}
	}()

	if client.Store.ID == nil {
		// No ID stored, new login
		qrChan, _ := client.GetQRChannel(context.Background())
		err = client.Connect()
		if err != nil {
			panic(err)
		}
		for evt := range qrChan {
			if evt.Event == "code" {
				ws.WriteJSON(map[string]string{
					"event": "code",
					"code":  evt.Code,
				})
				printCompactQR(evt.Code)
			} else {
				fmt.Println("Login event:", evt.Event)
			}
		}
	} else {
		err = client.Connect()
		if err != nil {
			ws.WriteJSON(map[string]string{"event": "error", "msg": "Erro ao reconectar"})
			return
		}
		ws.WriteJSON(map[string]string{"event": "restored", "msg": "Sessão restaurada com sucesso."})
	}

	select {}

}

func main() {
	http.HandleFunc("/ws", wsHandler)
	fmt.Println("Listening on :8080")
	http.ListenAndServe(":8080", nil)
}
