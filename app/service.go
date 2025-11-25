package app

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	db "github.com/simpplify-org/GO-simpzap/db/sqlc"
	"github.com/simpplify-org/GO-simpzap/pkg/whatsapp"
	"github.com/sqlc-dev/pqtype"
	"io"
	"net/http"
)

type WhatsAppServiceInterface interface {
	// Device Management
	CreateDeviceService(number string) (DeviceResponse, error)
	UpdateDeviceService(number string) (DeviceResponse, error)
	GetDeviceService(number string) (DeviceResponse, error)
	RemoveDeviceService(number string) error

	// Proxy (container child)
	ProxyService() http.Handler

	// Logs
	SaveEventLogService(arg InsertEventLogDTO) error
	ListLogsService(ctx context.Context, limit int32) ([]WhatsappEventLogResponse, error)
	ListLogsByNumberService(ctx context.Context, arg db.ListLogsByNumberParams) ([]WhatsappEventLogResponse, error)
}

type WhatsAppService struct {
	Zap  *whatsapp.ZapPkg
	Ctx  context.Context
	repo RepositoryInterface
}

func NewWhatsAppService(ctx context.Context, repo RepositoryInterface) *WhatsAppService {
	return &WhatsAppService{
		Zap:  whatsapp.NewZapPkg(),
		Ctx:  ctx,
		repo: repo,
	}
}

func (s *WhatsAppService) CreateDeviceService(number string) (DeviceResponse, error) {
	cc, err := s.Zap.CreateDevice(s.Ctx, number)
	if err != nil {
		return DeviceResponse{}, err
	}
	ID, err := s.repo.UpsertDeviceRepository(s.Ctx, db.UpsertDeviceParams{
		Number:      number,
		ContainerID: cc.ID,
		Endpoint:    cc.Endpoint,
		Version:     sql.NullString{String: cc.ImageTag, Valid: true},
		UpdatedWho: sql.NullString{
			String: "SYSTEM",
			Valid:  true,
		},
	})
	response := DeviceResponse{
		Status:        "created",
		Endpoint:      cc.Endpoint,
		IDContainer:   cc.ID,
		IDDevice:      ID,
		Version:       cc.ImageTag,
		VersionServer: s.Zap.GetVersion(),
	}
	if err != nil {
		return DeviceResponse{}, err
	}

	webhook, err := s.repo.ListWebhooksByDeviceRepository(s.Ctx, ID)
	if err != nil {
		return DeviceResponse{}, err
	}
	err = s.Zap.PushWebhooks(s.Ctx, cc, ToWebhookPKG(webhook))
	if err != nil {
		return DeviceResponse{}, err
	}

	return response, nil
}

func (s *WhatsAppService) UpdateDeviceService(number string) (DeviceResponse, error) {
	device, err := s.repo.GetDeviceRepository(s.Ctx, number)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return DeviceResponse{}, errors.New("device não encontrado")
		}
		return DeviceResponse{}, err
	}
	if device.Version.String == s.Zap.GetVersion() {
		return DeviceResponse{}, errors.New("device ja está atualizado")
	}
	err = s.Zap.RemoveDevice(s.Ctx, number)
	if err != nil {
		return DeviceResponse{}, err
	}
	cc, err := s.Zap.CreateDevice(s.Ctx, number)
	if err != nil {
		return DeviceResponse{}, err
	}
	ID, err := s.repo.UpsertDeviceRepository(s.Ctx, db.UpsertDeviceParams{
		Number:      number,
		ContainerID: cc.ID,
		Endpoint:    cc.Endpoint,
		Version:     sql.NullString{String: cc.ImageTag, Valid: true},
		UpdatedWho: sql.NullString{
			String: "SYSTEM",
			Valid:  true,
		},
	})
	response := DeviceResponse{
		Status:        "updated",
		Endpoint:      cc.Endpoint,
		IDContainer:   cc.ID,
		IDDevice:      ID,
		Version:       cc.ImageTag,
		VersionServer: s.Zap.GetVersion(),
	}
	if err != nil {
		return DeviceResponse{}, err
	}

	webhook, err := s.repo.ListWebhooksByDeviceRepository(s.Ctx, ID)
	if err != nil {
		return DeviceResponse{}, err
	}
	err = s.Zap.PushWebhooks(s.Ctx, cc, ToWebhookPKG(webhook))
	if err != nil {
		return DeviceResponse{}, err
	}

	return response, nil
}

func (s *WhatsAppService) GetDeviceService(number string) (DeviceResponse, error) {
	device, err := s.repo.GetDeviceRepository(s.Ctx, number)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return DeviceResponse{}, errors.New("device não encontrado")
		}
		return DeviceResponse{}, err
	}
	response := DeviceResponse{
		Status:        "getted",
		Endpoint:      device.Endpoint,
		IDContainer:   device.ContainerID,
		IDDevice:      device.ID,
		Version:       device.Version.String,
		VersionServer: s.Zap.GetVersion(),
	}
	return response, nil
}

func (s *WhatsAppService) RemoveDeviceService(number string) error {
	_, err := s.repo.GetDeviceRepository(s.Ctx, number)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("device não encontrado")
		}
		return err
	}

	err = s.repo.SoftDeleteDeviceRepository(s.Ctx, number)
	if err != nil {
		return err
	}

	return s.Zap.RemoveDevice(s.Ctx, number)
}

func (s *WhatsAppService) ProxyService() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/device/", func(w http.ResponseWriter, r *http.Request) {
		parts := splitPath(r.URL.Path)
		// /device/{device}/webhook/register
		//   0       1           2        3
		if len(parts) < 2 {
			http.Error(w, "device não informado", 400)
			return
		}
		device := parts[1]
		// --- Caso especial: registrar webhook ---
		if len(parts) >= 4 && parts[2] == "webhook" && parts[3] == "register" {
			var req struct {
				URL    string `json:"callback_url"`
				Method string `json:"url_method"`
				Body   string `json:"body"`
				Phrase string `json:"phrase"`
				Number string `json:"number"`
			}

			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "JSON inválido", 400)
				return
			}

			if req.URL == "" {
				http.Error(w, "callback_url obrigatório", 400)
				return
			}

			if req.Method == "" {
				req.Method = "POST"
			}
			deviceID, err := s.repo.GetDeviceRepository(r.Context(), device)
			if err != nil {
				http.Error(w, "erro ao registrar webhook: "+err.Error(), 500)
				return
			}

			err = s.repo.InsertWebhookRepository(r.Context(), db.InsertWebhookParams{
				DeviceID:  deviceID.ID,
				Number:    req.Number,
				Phrase:    req.Phrase,
				Url:       req.URL,
				UrlMethod: req.Method,
				Body: sql.NullString{
					String: req.Body,
					Valid:  req.Body != "",
				},
			})
			if err != nil {
				http.Error(w, "erro ao registrar webhook: "+err.Error(), 500)
				return
			}

			newBody, _ := json.Marshal(req)
			r.Body = io.NopCloser(bytes.NewReader(newBody))
			r.ContentLength = int64(len(newBody))
			r.Header.Set("Content-Length", fmt.Sprintf("%d", len(newBody)))
		}

		s.Zap.ProxyHandler().ServeHTTP(w, r)
	})
	return mux
}

func (s *WhatsAppService) SaveEventLogService(arg InsertEventLogDTO) error {
	return s.repo.InsertEventLogRepository(s.Ctx, db.InsertEventLogParams{
		Number:      arg.Number,
		Ip:          sql.NullString{String: arg.Ip, Valid: true},
		Method:      sql.NullString{String: arg.Method, Valid: true},
		Endpoint:    sql.NullString{String: arg.Endpoint, Valid: true},
		UserAgent:   sql.NullString{String: arg.UserAgent, Valid: true},
		StatusCode:  sql.NullString{String: arg.StatusCode, Valid: true},
		RequestBody: pqtype.NullRawMessage{RawMessage: arg.RequestBody, Valid: true},
	})
}

func (s *WhatsAppService) ListLogsService(ctx context.Context, limit int32) ([]WhatsappEventLogResponse, error) {
	result, err := s.repo.ListLastLogsRepository(ctx, limit)
	if err != nil {
		return nil, err
	}
	return ToLogResponse(result), err
}

func (s *WhatsAppService) ListLogsByNumberService(ctx context.Context, arg db.ListLogsByNumberParams) ([]WhatsappEventLogResponse, error) {
	result, err := s.repo.ListLogsByNumberRepository(ctx, arg)
	if err != nil {
		return nil, err
	}
	return ToLogResponse(result), err
}
