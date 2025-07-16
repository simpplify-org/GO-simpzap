package app

import (
	"log"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types/events"
)

type WhatsAppHandler struct {
	client *whatsmeow.Client
}

func NewWhatsAppHandler(client *whatsmeow.Client) *WhatsAppHandler {
	return &WhatsAppHandler{client: client}
}

var upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

func (h *WhatsAppHandler) RegisterRoutes(e *echo.Echo) {
	ws := e.Group("/ws", checkAuthorization)
	ws.GET("/whatsapp", h.WsHandler)
	ws.POST("/send", h.SendMessageHandler)
	ws.GET("/status", h.StatusHandler)
}

func (h *WhatsAppHandler) SendMessageHandler(c echo.Context) error {
	var req SendMessageRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	if err := SendMessage(h.client, req.To, req.Text); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "mensagem enviada com sucesso"})
}

func (h *WhatsAppHandler) StatusHandler(c echo.Context) error {
	status := "desconectado"
	if h.client.Store.ID != nil && h.client.IsConnected() {
		status = "conectado"
	}
	return c.JSON(http.StatusOK, map[string]string{"status": status})
}

func (h *WhatsAppHandler) WsHandler(c echo.Context) error {
	ws, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		log.Println(err)
		return err
	}
	defer ws.Close()

	client := h.client

	client.AddEventHandler(func(evt interface{}) {
		switch v := evt.(type) {
		case *events.Disconnected:
			ws.WriteJSON(map[string]string{"event": "disconnected", "msg": "Desconectado. Escaneie o QR Code novamente."})
		case *events.Connected:
			ws.WriteJSON(map[string]string{"event": "connected", "msg": "Conectado com sucesso!"})
		default:
			_ = v
		}
	})

	type IncomingMessage struct {
		Event string `json:"event"`
		To    string `json:"to"`
		Text  string `json:"text"`
	}

	for {
		var msg IncomingMessage
		err := ws.ReadJSON(&msg)
		if err != nil {
			log.Println("Erro ao ler mensagem do cliente:", err)
			break
		}
		if msg.Event == "send_message" {
			err := SendMessage(client, msg.To, msg.Text)
			if err != nil {
				ws.WriteJSON(map[string]string{"event": "send_error", "msg": "Erro ao enviar mensagem: " + err.Error()})
			} else {
				ws.WriteJSON(map[string]string{"event": "message_sent", "msg": "Mensagem enviada para " + msg.To})
			}
		}
	}

	return nil
}
