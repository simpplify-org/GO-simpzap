package app

import (
	"encoding/json"
	db "github.com/simpplify-org/GO-simpzap/db/sqlc"
	pkg "github.com/simpplify-org/GO-simpzap/pkg/whatsapp"
	"time"
)

type CreateDeviceRequest struct {
	Number string `json:"number" validate:"required"`
}

type DeleteDeviceRequest struct {
	Number string `json:"number" validate:"required"`
}

type CreateDeviceResponse struct {
	Status   string `json:"status"`
	Endpoint string `json:"endpoint"`
	ID       string `json:"id"`
	Version  string `json:"version"`
}

type DeleteDeviceResponse struct {
	Status string `json:"status"`
}

type InsertEventLogDTO struct {
	Number      string          `json:"number"`
	Ip          string          `json:"ip"`
	Method      string          `json:"method"`
	Endpoint    string          `json:"endpoint"`
	UserAgent   string          `json:"user_agent"`
	StatusCode  string          `json:"status_code"`
	RequestBody json.RawMessage `json:"request_body"`
}

func ToWebhookPKG(arg []db.WhatsappDeviceWebhook) []pkg.WebhookRegister {
	var result []pkg.WebhookRegister
	for _, webhook := range arg {
		result = append(result, pkg.WebhookRegister{
			Phrase:      webhook.Phrase,
			CallbackURL: webhook.Url,
			UrlMethod:   webhook.UrlMethod,
			Number:      webhook.Number,
			Body:        webhook.Body.String,
		})
	}
	return result
}

type WhatsappEventLogResponse struct {
	ID          int64           `json:"id"`
	Number      string          `json:"number"`
	Ip          string          `json:"ip"`
	Method      string          `json:"method"`
	Endpoint    string          `json:"endpoint"`
	UserAgent   string          `json:"user_agent"`
	StatusCode  string          `json:"status_code"`
	RequestBody json.RawMessage `json:"request_body"`
	CreatedAt   time.Time       `json:"created_at"`
}

func ToLogResponse(arg []db.WhatsappEventLog) []WhatsappEventLogResponse {
	var result []WhatsappEventLogResponse
	for _, a := range arg {
		result = append(result, WhatsappEventLogResponse{
			ID:          a.ID,
			Number:      a.Number,
			Ip:          a.Ip.String,
			Method:      a.Method.String,
			Endpoint:    a.Endpoint.String,
			UserAgent:   a.UserAgent.String,
			StatusCode:  a.StatusCode.String,
			RequestBody: a.RequestBody.RawMessage,
			CreatedAt:   a.CreatedAt,
		})
	}
	return result
}
