package app

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

type WhatsAppHandler struct {
	Service   *WhatsAppService
	Auth      *AuthHandler
	DashHTML  []byte
	LoginHTML []byte
	// APIKeyMiddleware protege /create, /devices, /delete e /device/*. É
	// no-op se DEVICE_API_KEY não estiver configurada (ver cmd/main.go).
	APIKeyMiddleware echo.MiddlewareFunc
}

func NewWhatsAppHandler(svc *WhatsAppService, auth *AuthHandler) *WhatsAppHandler {
	return &WhatsAppHandler{
		Service: svc,
		Auth:    auth,
		// no-op por padrão; cmd/main.go sobrescreve com a chave real, se houver.
		APIKeyMiddleware: NewAPIKeyMiddleware(""),
	}
}

func (h *WhatsAppHandler) RegisterRoutes(e *echo.Echo) {
	e.GET("/login", h.Login)
	e.POST("/login", h.Auth.Login)
	e.POST("/logout", h.Auth.Logout)
	e.POST("/users", h.Auth.CreateUser, h.Auth.RequireAuth)

	e.GET("/dash", h.Dash, h.Auth.RequireAuth)
	e.POST("/create", h.CreateDevice, h.APIKeyMiddleware)
	e.GET("/devices", h.ListDevices, h.APIKeyMiddleware)
	e.DELETE("/delete", h.DeleteDevice, h.APIKeyMiddleware)
	e.Any("/device/*", echo.WrapHandler(h.Service.ProxyHandler()), h.APIKeyMiddleware) //DIRECIONA PARA O CONTAINER CHILD
}

func (h *WhatsAppHandler) Dash(c echo.Context) error {
	if len(h.DashHTML) == 0 {
		return c.String(http.StatusNotFound, "dashboard not available")
	}
	return c.HTMLBlob(http.StatusOK, h.DashHTML)
}

func (h *WhatsAppHandler) Login(c echo.Context) error {
	if len(h.LoginHTML) == 0 {
		return c.String(http.StatusNotFound, "login not available")
	}
	return c.HTMLBlob(http.StatusOK, h.LoginHTML)
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

func (h *WhatsAppHandler) ListDevices(c echo.Context) error {
	devices, err := h.Service.ListDevices()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
	}
	return c.JSON(http.StatusOK, devices)
}
