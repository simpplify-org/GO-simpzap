package app

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

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
	e.POST("/send/many", h.SendBulkMessage)
	e.GET("/ws/create", h.HandleWebSocketCreate, checkAuthorization)
	e.GET("/ws/create/nt", h.HandleWebSocketCreateNew)
	e.GET("/check/status/:device_id", h.GetSessionStatus)
	e.GET("/connect", h.HandleWebSocketConnect)
	e.GET("/list/devices", h.GetDevices)
	e.POST("/contacts/create", h.InsertListContact)
	e.GET("/contacts/list/:device_id", h.ListContacts)
	e.DELETE("/contacts/delete/:device_id", h.DeleteContact)
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

	existingDevice, err := h.Service.FindDeviceByTenantAndNumber(context.Background(), tenantID, number)
	var deviceID string
	var sessionPath string

	if err == nil && existingDevice != nil {
		deviceID = existingDevice.ID.Hex()
		sessionPath = string(existingDevice.SessionDB)
		ws.WriteJSON(map[string]string{
			"event":     "existing_device",
			"device_id": deviceID,
		})
	} else {

		deviceID = uuid.NewString()
	}

	client, qrChan, newSessionPath, err := whatsapp.CreateClient(deviceID)
	if err != nil {
		ws.WriteJSON(map[string]string{"error": err.Error()})
		return nil
	}

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
				"msg":   "Desconectado. Tentando reconectar...",
			})

			go func() {
				err := client.Connect()
				if err != nil {
					ws.WriteJSON(map[string]string{"Erro ao reconectar": err.Error()})
					log.Println("Erro ao reconectar:", err)
				}
			}()

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
			//ws.Close()
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
		default:
			_ = v
		}
	})

	return nil
}

func (h *WhatsAppHandler) HandleWebSocketConnect(c echo.Context) error {
	tenantID := c.QueryParam("tenant_id")
	number := c.QueryParam("number")
	if tenantID == "" || number == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "tenant_id e number são obrigatórios")
	}
	ws, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return err
	}
	device, err := h.Service.FindDeviceByTenantAndNumber(context.Background(), tenantID, number)
	if err != nil {
		ws.WriteJSON(map[string]string{"error": err.Error(), "message": "Dispositivo não encontrado"})
		ws.Close()
		return nil
	}

	if len(device.ID) == 0 {
		ws.WriteJSON(map[string]string{
			"event":   "disconnected",
			"message": "Nenhuma sessão salva",
		})
		ws.Close()
		return nil
	}

	client, _, err := whatsapp.StartClient(device.SessionDB)
	if err != nil {
		ws.WriteJSON(map[string]string{"status": "error", "message": "Erro ao restaurar sessão"})
		ws.Close()
		return nil
	}

	client.AddEventHandler(func(evt interface{}) {
		switch v := evt.(type) {
		case *events.Connected:
			_ = h.Service.UpdateDeviceConnectionStatus(context.Background(), device.ID, true)
			ws.WriteJSON(map[string]interface{}{
				"status":    "connected",
				"device_id": device.ID.Hex(),
				"number":    device.Number,
				"tenant_id": device.TenantID,
			})
		case *events.Disconnected:
			_ = h.Service.UpdateDeviceConnectionStatus(context.Background(), device.ID, false)
			ws.WriteJSON(map[string]interface{}{
				"status":    "disconnected",
				"device_id": device.ID.Hex(),
				"number":    device.Number,
				"tenant_id": device.TenantID,
			})
		default:
			_ = v
		}
	})

	return nil
}

func (h *WhatsAppHandler) GetDevices(c echo.Context) error {
	var response []DeviceResponse
	tenantID := c.QueryParam("tenant_id")
	if tenantID == "" {
		return c.JSON(http.StatusBadRequest, "tenant_id é obrigatório")
	}

	devices, err := h.Service.GetAllDevices(context.Background(), tenantID)
	if err != nil {
		fmt.Printf("Erro ao buscar dispositivos: %v\n", err)
		return c.JSON(http.StatusInternalServerError, err.Error())
	}

	for _, device := range devices {
		response = append(response, DeviceResponse{
			ID:        device.ID.Hex(),
			TenantID:  device.TenantID,
			Number:    device.Number,
			Connected: device.Connected,
			CreatedAt: device.CreatedAt,
		})
	}

	return c.JSON(http.StatusOK, response)
}

func (h *WhatsAppHandler) SendBulkMessage(c echo.Context) error {
	var req SendBulkMessageRequest

	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, err.Error())
	}

	if len(req.Numbers) <= 0 {
		return c.JSON(http.StatusBadRequest, "nenhum número informado")
	}

	if len(req.Numbers) > 100 {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Máximo de 100 números por requisição",
		})
	}

	err := h.Service.SendManyMessages(req.DeviceID, req.Numbers, req.Message)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, map[string]string{
		"status":  "sent",
		"sent_to": strconv.Itoa(len(req.Numbers)),
	})
}

func (h *WhatsAppHandler) InsertListContact(c echo.Context) error {
	var req ContactListRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, err.Error())
	}

	result, err := h.Service.InsertListContact(context.Background(), req)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusCreated, map[string]interface{}{
		"id": result.InsertedID,
	})
}

func (h *WhatsAppHandler) ListContacts(c echo.Context) error {
	var response []ContactListResponse

	deviceID := c.Param("device_id")

	contacts, err := h.Service.ListContacts(context.Background(), deviceID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, err.Error())
	}

	for _, contact := range contacts {
		response = append(response, ContactListResponse{
			ID:        contact.ID,
			DeviceID:  contact.DeviceID,
			Name:      contact.Name,
			Number:    contact.Number,
			CreatedAt: contact.CreatedAt,
		})
	}

	return c.JSON(http.StatusOK, response)
}

func (h *WhatsAppHandler) DeleteContact(c echo.Context) error {
	id := c.Param("id")

	err := h.Service.DeleteContact(context.Background(), id)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"status": "deleted",
	})
}
