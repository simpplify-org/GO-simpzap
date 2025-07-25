package app

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
	"github.com/mdp/qrterminal/v3"
	"github.com/simpplify-org/GO-simpzap/pkg/whatsapp"
	"go.mau.fi/whatsmeow/types/events"
	"net/http"
	"os"
)

type WhatsAppHandler struct {
	Service *WhatsAppService
}

func NewWhatsAppHandler(s *WhatsAppService) *WhatsAppHandler {
	return &WhatsAppHandler{Service: s}
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func (h *WhatsAppHandler) RegisterRoutes(e *echo.Echo) {
	e.POST("/send", h.SendMessage, checkAuthorization)
	e.GET("/ws/create", h.HandleWebSocketCreate, checkAuthorization)
	//e.GET("/ws/:device_id", h.WebSocketConnection, checkAuthorization)
}

func (h *WhatsAppHandler) SendMessage(c echo.Context) error {
	var req SendMessageRequest
	if err := c.Bind(&req); err != nil {
		return err
	}

	err := h.Service.SendMessage(req.DeviceID, req.Number, req.Message)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return c.JSON(http.StatusOK, map[string]string{"status": "sent"})
}

func (h *WhatsAppHandler) HandleWebSocketCreate(c echo.Context) error {
	tenantID := c.QueryParam("tenant_id")
	number := c.QueryParam("number")

	if tenantID == "" || number == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "tenant_id e number são obrigatórios")
	}

	ws, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return err
	}
	//defer ws.Close()

	client, qrChan, sessionPath, err := whatsapp.CreateClient(uuid.NewString())
	if err != nil {
		ws.WriteJSON(map[string]string{"error": err.Error()})
		return nil
	}

	if qrChan != nil {
		for evt := range qrChan {
			if evt.Event == "code" {
				ws.WriteJSON(map[string]string{
					"event": "code",
					"code":  evt.Code,
				})
				qr := evt.Code
				fmt.Println("Scan this QR code to log in:")
				qrterminal.GenerateHalfBlock(qr, qrterminal.L, os.Stdout)
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
			ctx := context.Background()
			device, err := h.Service.SaveConnectedDevice(ctx, tenantID, number, sessionPath)
			if err != nil {
				ws.WriteJSON(map[string]string{"error": err.Error()})
				return
			}
			ws.WriteJSON(map[string]string{
				"status":    "connected",
				"device_id": device.ID.Hex(),
				"number":    device.Number,
				"tenant_id": device.TenantID,
			})
			ws.Close()
		default:
			_ = v
		}
	})

	return nil
}
