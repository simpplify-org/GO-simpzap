package app

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

type WhatsAppHandler struct {
	Service *WhatsAppService
}

func NewWhatsAppHandler(svc *WhatsAppService) *WhatsAppHandler {
	return &WhatsAppHandler{Service: svc}
}

func (h *WhatsAppHandler) RegisterRoutes(e *echo.Echo) {
	e.POST("/create", h.CreateDevice)
	e.DELETE("/delete", h.DeleteDevice)
	e.Any("/device/*", echo.WrapHandler(h.Service.ProxyHandler())) //DIRECIONA PARA O CONTAINER CHILD
}

func (h *WhatsAppHandler) CreateDevice(c echo.Context) error {
	var req CreateDeviceRequest
	if err := c.Bind(&req); err != nil || req.Number == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "JSON inválido, envie {\"number\": \"5511999999999\"}",
		})
	}

	resp, err := h.Service.CreateDevice(req.Number)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
	}
	return c.JSON(http.StatusCreated, resp)
}

func (h *WhatsAppHandler) DeleteDevice(c echo.Context) error {
	var req DeleteDeviceRequest
	if err := c.Bind(&req); err != nil || req.Number == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "JSON inválido"})
	}

	if err := h.Service.RemoveDevice(req.Number); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	resp := DeleteDeviceResponse{Status: "removed"}
	return c.JSON(http.StatusOK, resp)
}
