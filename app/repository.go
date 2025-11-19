package app

import (
	"context"
	"database/sql"

	"github.com/simpplify-org/GO-simpzap/db/sqlc"
)

type RepositoryInterface interface {
	// DEVICE
	UpsertDevice(ctx context.Context, arg db.UpsertDeviceParams) (int64, error)
	UpdateDevice(ctx context.Context, arg db.UpdateDeviceParams) error
	SoftDeleteDevice(ctx context.Context, number string) error
	GetDevice(ctx context.Context, number string) (db.WhatsappDevice, error)
	GetDevices(ctx context.Context) ([]db.WhatsappDevice, error)

	// WEBHOOK
	InsertWebhook(ctx context.Context, arg db.InsertWebhookParams) error
	ListWebhooksByDevice(ctx context.Context, deviceID int64) ([]db.WhatsappDeviceWebhook, error)
	GetWebhookByPhrase(ctx context.Context, arg db.GetWebhookByPhraseParams) (db.WhatsappDeviceWebhook, error)
	SoftDeleteWebhook(ctx context.Context, id int64) error

	// EVENT LOG
	InsertEventLog(ctx context.Context, arg db.InsertEventLogParams) error
	ListLogsByNumber(ctx context.Context, arg db.ListLogsByNumberParams) ([]db.WhatsappEventLog, error)
	ListLastLogs(ctx context.Context, limit int32) ([]db.WhatsappEventLog, error)
}

type Repository struct {
	db      *sql.DB
	queries *db.Queries
}

func NewRepository(database *sql.DB) RepositoryInterface {
	return &Repository{
		db:      database,
		queries: db.New(database),
	}
}

func (r *Repository) UpsertDevice(ctx context.Context, arg db.UpsertDeviceParams) (int64, error) {
	return r.queries.UpsertDevice(ctx, arg)
}

func (r *Repository) UpdateDevice(ctx context.Context, arg db.UpdateDeviceParams) error {
	return r.queries.UpdateDevice(ctx, arg)
}

func (r *Repository) SoftDeleteDevice(ctx context.Context, number string) error {
	return r.queries.SoftDeleteDevice(ctx, number)
}

func (r *Repository) GetDevice(ctx context.Context, number string) (db.WhatsappDevice, error) {
	return r.queries.GetDevice(ctx, number)
}

func (r *Repository) GetDevices(ctx context.Context) ([]db.WhatsappDevice, error) {
	return r.queries.GetDevices(ctx)
}

func (r *Repository) InsertWebhook(ctx context.Context, arg db.InsertWebhookParams) error {
	return r.queries.InsertWebhook(ctx, arg)
}

func (r *Repository) ListWebhooksByDevice(ctx context.Context, device int64) ([]db.WhatsappDeviceWebhook, error) {
	return r.queries.ListWebhooksByDevice(ctx, device)
}

func (r *Repository) GetWebhookByPhrase(ctx context.Context, arg db.GetWebhookByPhraseParams) (db.WhatsappDeviceWebhook, error) {
	return r.queries.GetWebhookByPhrase(ctx, arg)
}

func (r *Repository) SoftDeleteWebhook(ctx context.Context, id int64) error {
	return r.queries.SoftDeleteWebhook(ctx, id)
}

func (r *Repository) InsertEventLog(ctx context.Context, arg db.InsertEventLogParams) error {
	return r.queries.InsertEventLog(ctx, arg)
}

func (r *Repository) ListLogsByNumber(ctx context.Context, arg db.ListLogsByNumberParams) ([]db.WhatsappEventLog, error) {
	return r.queries.ListLogsByNumber(ctx, arg)
}

func (r *Repository) ListLastLogs(ctx context.Context, limit int32) ([]db.WhatsappEventLog, error) {
	return r.queries.ListLastLogs(ctx, limit)
}
