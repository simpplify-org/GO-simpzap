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

	CreateDeviceHandler(c echo.Context) error
	DeleteDeviceHandler(c echo.Context) error

	ListLogsHandler(c echo.Context) error
	ListLogsByNumberHandler(c echo.Context) error

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
	e.POST("/create", h.CreateDeviceHandler)
	e.DELETE("/delete", h.DeleteDeviceHandler)
	e.Any("/device/*", echo.WrapHandler(h.Service.ProxyService()))
	e.GET("/metrics", echo.WrapHandler(promhttp.Handler()))
	e.GET("/logs", h.ListLogsHandler)
	e.GET("/logs/number", h.ListLogsByNumberHandler)
}

func (h *WhatsAppHandler) CreateDeviceHandler(c echo.Context) error {
	var req CreateDeviceRequest
	if err := c.Bind(&req); err != nil || req.Number == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "JSON inválido, envie {\"number\": \"5511999999999\"}",
		})
	}

	resp, err := h.Service.CreateDeviceService(req.Number)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": err.Error(),
		})
	}
	return c.JSON(http.StatusCreated, resp)
}

func (h *WhatsAppHandler) DeleteDeviceHandler(c echo.Context) error {
	var req DeleteDeviceRequest
	if err := c.Bind(&req); err != nil || req.Number == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "JSON inválido"})
	}

	if err := h.Service.RemoveDeviceService(req.Number); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	resp := DeleteDeviceResponse{Status: "removed"}
	return c.JSON(http.StatusOK, resp)
}

func (h *WhatsAppHandler) ListLogsHandler(c echo.Context) error {
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
	result, err := h.Service.ListLogsService(c.Request().Context(), limit)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}

	return c.JSON(http.StatusOK, result)
}

func (h *WhatsAppHandler) ListLogsByNumberHandler(c echo.Context) error {
	var req db.ListLogsByNumberParams
	if err := c.Bind(&req); err != nil || req.Number == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "JSON inválido"})
	}
	result, err := h.Service.ListLogsByNumberService(c.Request().Context(), req)
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

		if saveErr := h.Service.SaveEventLogService(log); saveErr != nil {
			c.Logger().Error("Erro ao salvar logs:", saveErr)
		}

		return err
	}
}
