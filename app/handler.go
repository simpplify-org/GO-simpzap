package app

import (
	"context"
	"fmt"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"net/http"
	"os"
	"strings"

	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
	"github.com/mdp/qrterminal/v3"
	"github.com/simpplify-org/GO-data-connector-lib/slack"
	"github.com/simpplify-org/GO-simpzap/pkg/whatsapp"
	"github.com/skip2/go-qrcode"
	"go.mau.fi/whatsmeow/types/events"
)

type WhatsAppHandler struct {
	Service  *WhatsAppService
	Reporter *slack.Reporter
}

func NewWhatsAppHandler(s *WhatsAppService, r *slack.Reporter) *WhatsAppHandler {
	return &WhatsAppHandler{Service: s, Reporter: r}
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func (h *WhatsAppHandler) RegisterRoutes(e *echo.Echo) {
	e.POST("/send", h.SendMessage, checkAuthorization)
	e.POST("/send/nt", h.SendMessage)
	e.GET("/ws/create", h.HandleWebSocketCreate, checkAuthorization)
	e.GET("/ws/create/nt", h.HandleWebSocketCreateNew)
	e.GET("/check/status/:device_id", h.GetSessionStatus)
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

	// Verifica se já existe um dispositivo para este tenant/number
	existingDevice, err := h.Service.FindDeviceByTenantAndNumber(context.Background(), tenantID, number)
	var deviceID string
	var sessionPath string

	if err == nil && existingDevice != nil {
		// Dispositivo existe, usa o ID e sessionPath existentes
		deviceID = existingDevice.ID.Hex()
		sessionPath = string(existingDevice.SessionDB)
		ws.WriteJSON(map[string]string{
			"event":     "existing_device",
			"device_id": deviceID,
		})
	} else {
		// Cria novo dispositivo
		deviceID = uuid.NewString()
	}

	// Cria/restaura o cliente
	client, qrChan, newSessionPath, err := whatsapp.CreateClient(deviceID)
	if err != nil {
		ws.WriteJSON(map[string]string{"error": err.Error()})
		return nil
	}

	// Atualiza o sessionPath se for um novo dispositivo
	if existingDevice == nil {
		sessionPath = newSessionPath
	}

	isNoToken := strings.HasSuffix(c.Path(), "/nt")

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

				if isNoToken && h.Reporter != nil {
					if err := sendQRCodeToSlack(evt.Code, h.Reporter); err != nil {
						fmt.Println("Erro ao enviar QR Code para o Slack:", err)
					}
				}
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
		ctx := context.Background()
		device, err := h.Service.SaveConnectedDevice(ctx, tenantID, number, sessionPath)
		if err != nil {
			ws.WriteJSON(map[string]string{"error": err.Error()})
			return
		}
		switch v := evt.(type) {
		case *events.Disconnected:
			if device != nil {
				_ = h.Service.UpdateDeviceConnectionStatus(context.Background(), device.ID, false)
			}
			ws.WriteJSON(map[string]string{
				"event": "disconnected",
				"msg":   "Desconectado. Escaneie o QR Code novamente.",
			})
		case *events.Connected:
			if device != nil {
				_ = h.Service.UpdateDeviceConnectionStatus(context.Background(), device.ID, true)
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

func (h *WhatsAppHandler) GetSessionStatus(c echo.Context) error {
	deviceIDStr := c.Param("device_id")
	if deviceIDStr == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "device_id obrigatório"})
	}

	deviceID, err := primitive.ObjectIDFromHex(deviceIDStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "device_id inválido"})
	}

	var device Device
	err = h.Service.Repo.Collection.FindOne(context.Background(), bson.M{"_id": deviceID}).Decode(&device)
	if err != nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "dispositivo não encontrado"})
	}

	return c.JSON(http.StatusOK, map[string]bool{"is_active": device.Connected})
}

func sendQRCodeToSlack(codeRaw string, reporter *slack.Reporter) error {
	filePath := fmt.Sprintf("%s/qrcode-%d.png", os.TempDir(), time.Now().Unix())

	if err := qrcode.WriteFile(codeRaw, qrcode.Medium, 256, filePath); err != nil {
		return fmt.Errorf("erro ao gerar QR Code: %w", err)
	}
	defer os.Remove(filePath)

	return reporter.SendImageToSlack(filePath, "QR Code WhatsApp")
}

func (s *WhatsAppService) UpdateDeviceConnectionStatus(ctx context.Context, deviceID primitive.ObjectID, connected bool) error {
	filter := bson.M{"_id": deviceID}
	update := bson.M{
		"$set": bson.M{
			"connected":  connected,
			"updated_at": time.Now().Unix(),
		},
	}
	_, err := s.Repo.Collection.UpdateOne(ctx, filter, update)
	return err
}

func (h *WhatsAppHandler) HandleWebSocketCreateNew(c echo.Context) error {
	tenantID := c.QueryParam("tenant_id")
	number := c.QueryParam("number")

	if tenantID == "" || number == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "tenant_id e number são obrigatórios")
	}

	ws, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return err
	}

	// Verifica se já existe um dispositivo
	existingDevice, err := h.Service.FindDeviceByTenantAndNumber(context.Background(), tenantID, number)
	var deviceID string

	if existingDevice != nil {
		deviceID = existingDevice.ID.Hex()
		ws.WriteJSON(map[string]string{
			"event":     "existing_device",
			"device_id": deviceID,
		})
	} else {
		deviceID = uuid.NewString()
	}

	// Cria/restaura o cliente
	client, qrChan, sessionPath, err := whatsapp.CreateClient(deviceID)
	if err != nil {
		ws.WriteJSON(map[string]string{"error": err.Error()})
		return nil
	}

	isNoToken := strings.HasSuffix(c.Path(), "/nt")

	if qrChan != nil {
		for evt := range qrChan {
			if evt.Event == "code" {
				ws.WriteJSON(map[string]string{
					"event": "code",
					"code":  evt.Code,
				})

				if isNoToken && h.Reporter != nil {
					if err := sendQRCodeToSlack(evt.Code, h.Reporter); err != nil {
						fmt.Println("Erro ao enviar QR Code para o Slack:", err)
					}
				}
			}
		}
	} else if existingDevice == nil {
		// Se não tem QR chan e é um novo dispositivo, envia mensagem de sucesso
		ws.WriteJSON(map[string]string{
			"event": "restored_new",
			"msg":   "Sessão criada com sucesso.",
		})
	}

	client.AddEventHandler(func(evt interface{}) {
		device, err := h.Service.SaveConnectedDevice(context.Background(), tenantID, number, sessionPath)
		if err != nil {
			ws.WriteJSON(map[string]string{"error": err.Error()})
			return
		}

		switch v := evt.(type) {
		case *events.Disconnected:
			_ = h.Service.UpdateDeviceConnectionStatus(context.Background(), device.ID, false)
			ws.WriteJSON(map[string]string{
				"event": "disconnected",
				"msg":   "Desconectado. Escaneie o QR Code novamente.",
			})
		case *events.Connected:
			_ = h.Service.UpdateDeviceConnectionStatus(context.Background(), device.ID, true)
			ws.WriteJSON(map[string]interface{}{
				"status":    "connected",
				"device_id": device.ID.Hex(),
				"number":    device.Number,
				"tenant_id": device.TenantID,
				"is_new":    existingDevice == nil,
			})
			ws.Close()
		default:
			_ = v
		}
	})

	return nil
}
