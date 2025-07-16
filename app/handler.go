package app

import (
	"context"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
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
	colorReset = "\033[0m"
	colorGreen = "\033[32m"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
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

var GlobalClient *whatsmeow.Client

func GetClient() *whatsmeow.Client {
	return GlobalClient
}

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
		qrChan, _ = client.GetQRChannel(context.Background())
	}

	err = client.Connect()
	if err != nil {
		return nil, nil, fmt.Errorf("Erro ao conectar client: %w", err)
	}

	GlobalClient = client
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

func WsHandler(c echo.Context) error {
	ws, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		log.Println(err)
		return err
	}
	fmt.Println(colorGreen + "WebSocket Connected" + colorReset)
	defer ws.Close()

	client, qrChan, err := InitWhatsAppClient()
	if err != nil {
		log.Println(err)
		ws.WriteJSON(map[string]string{
			"event": "error",
			"msg":   err.Error(),
		})
		return err
	}
	if qrChan != nil {
		for evt := range qrChan {
			if evt.Event == "code" {
				ws.WriteJSON(map[string]string{
					"event": "code",
					"code":  evt.Code,
				})
				PrintCompactQR(evt.Code)
			} else {
				fmt.Println("Login event:", evt.Event)
			}
		}
	} else {
		ws.WriteJSON(map[string]string{
			"event": "restored",
			"msg":   "Sessão restaurada com sucesso.",
		})
	}

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
				err := SendMessage(client, msg.To, msg.Text)
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
				PrintCompactQR(evt.Code)
			} else {
				fmt.Println("Login event:", evt.Event)
			}
		}
	} else {
		err = client.Connect()
		if err != nil {
			ws.WriteJSON(map[string]string{"event": "error", "msg": "Erro ao reconectar"})
			return err
		}
		ws.WriteJSON(map[string]string{"event": "restored", "msg": "Sessão restaurada com sucesso."})
	}

	select {}
}

func SendMessageHandler(c echo.Context) error {
	var req SendMessageRequest

	client, qrChan, err := InitWhatsAppClient()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{"error": err.Error()})
	}
	if qrChan != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{"status": "simppzap is not initialized yet"})
	}
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	if err := SendMessage(client, req.To, req.Text); err != nil {
		return c.JSON(http.StatusInternalServerError, echo.Map{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, echo.Map{"status": "mensagem enviada com sucesso"})
}

func StatusHandler(c echo.Context) error {
	status := "desconectado"
	if GlobalClient.Store.ID != nil && GlobalClient.IsConnected() {
		status = "conectado"
	}
	return c.JSON(http.StatusOK, map[string]string{"status": status})
}
