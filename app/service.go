package app

import (
	"context"
	"github.com/simpplify-org/GO-simpzap/pkg/whatsapp"
	"net/http"
)

type WhatsAppService struct {
	Zap *whatsapp.ZapPkg
	Ctx context.Context
}

func NewWhatsAppService(ctx context.Context) *WhatsAppService {
	return &WhatsAppService{
		Zap: whatsapp.NewZapPkg(),
		Ctx: ctx,
	}
}

func (s *WhatsAppService) CreateDevice(number string) (CreateDeviceResponse, error) {
	cc, err := s.Zap.CreateDevice(s.Ctx, number)
	if err != nil {
		return CreateDeviceResponse{}, err
	}
	response := CreateDeviceResponse{
		Status:   "created",
		Endpoint: cc.Endpoint,
		ID:       cc.ID,
	}
	return response, nil
}

func (s *WhatsAppService) RemoveDevice(number string) error {
	return s.Zap.RemoveDevice(s.Ctx, number)
}

func (s *WhatsAppService) ProxyHandler() http.Handler {
	return s.Zap.ProxyHandler()
}
