package app

import (
	"bytes"
	"fmt"
	db "github.com/simpplify-org/GO-simpzap/db/sqlc"
	"io"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type WhatsAppHandlerInterface interface {
	RegisterRoutes(e *echo.Echo) //CHAMADO INTERNAMENTE SOMENTE

	CreateDevice(c echo.Context) error
	DeleteDevice(c echo.Context) error

	ListLogs(c echo.Context) error
	ListLogsByNumber(c echo.Context) error

	EventLoggerMiddleware(next echo.HandlerFunc) echo.HandlerFunc //CHAMADO INTERNAMENTE SOMENTE
}

type WhatsAppHandler struct {
	Service *WhatsAppService
}

func NewWhatsAppHandler(svc *WhatsAppService) *WhatsAppHandler {
	return &WhatsAppHandler{Service: svc}
}

func (h *WhatsAppHandler) RegisterRoutes(e *echo.Echo) {
	e.Use(h.EventLoggerMiddleware)
	e.POST("/create", h.CreateDevice)
	e.DELETE("/delete", h.DeleteDevice)
	e.Any("/device/*", echo.WrapHandler(h.Service.ProxyHandler()))
	e.GET("/metrics", echo.WrapHandler(promhttp.Handler()))
	e.GET("/logs", h.ListLogs)
	e.GET("/logs/number", h.ListLogsByNumber)
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

func (h *WhatsAppHandler) ListLogs(c echo.Context) error {
	limitStr := c.QueryParam("limit")
	if limitStr == "" {
		limitStr = "10"
	}
	limitInt, err := strconv.Atoi(limitStr)
	if err != nil || limitInt <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "limit inválido, deve ser um número inteiro positivo",
		})
	}

	limit := int32(limitInt)
	result, err := h.Service.ListLogs(c.Request().Context(), limit)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, result)
}

func (h *WhatsAppHandler) ListLogsByNumber(c echo.Context) error {
	var req db.ListLogsByNumberParams
	if err := c.Bind(&req); err != nil || req.Number == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "JSON inválido"})
	}
	result, err := h.Service.ListLogsByNumber(c.Request().Context(), req)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, result)
}

func (h *WhatsAppHandler) EventLoggerMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		req := c.Request()
		res := c.Response()

		var bodyBytes []byte
		if c.Path() == "/create" || c.Path() == "/delete" {
			if req.Body != nil {
				bodyBytes, _ = io.ReadAll(req.Body)
				req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
			}
		}

		number := extractNumber(c, bodyBytes)
		err := next(c)

		raw := string(bodyBytes)
		log := InsertEventLogDTO{
			Number:      number,
			Ip:          c.RealIP(),
			Method:      req.Method,
			Endpoint:    req.URL.Path,
			UserAgent:   req.UserAgent(),
			StatusCode:  fmt.Sprintf("%d", res.Status),
			RequestBody: []byte(fmt.Sprintf("%q", raw)),
		}

		if saveErr := h.Service.SaveEventLog(log); saveErr != nil {
			c.Logger().Error("Erro ao salvar logs:", saveErr)
		}

		return err
	}
}
