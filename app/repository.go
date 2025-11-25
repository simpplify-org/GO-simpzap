package app

import (
	"context"
	"database/sql"

	"github.com/simpplify-org/GO-simpzap/db/sqlc"
)

type RepositoryInterface interface {
	// DEVICE
	UpsertDeviceRepository(ctx context.Context, arg db.UpsertDeviceParams) (int64, error)
	SoftDeleteDeviceRepository(ctx context.Context, number string) error
	GetDeviceRepository(ctx context.Context, number string) (db.WhatsappDevice, error)
	GetDevicesRepository(ctx context.Context) ([]db.WhatsappDevice, error)

	// WEBHOOK
	InsertWebhookRepository(ctx context.Context, arg db.InsertWebhookParams) error
	ListWebhooksByDeviceRepository(ctx context.Context, deviceID int64) ([]db.WhatsappDeviceWebhook, error)
	GetWebhookByPhraseRepository(ctx context.Context, arg db.GetWebhookByPhraseParams) (db.WhatsappDeviceWebhook, error)
	SoftDeleteWebhookRepository(ctx context.Context, id int64) error

	// EVENTLOG
	InsertEventLogRepository(ctx context.Context, arg db.InsertEventLogParams) error
	ListLogsByNumberRepository(ctx context.Context, arg db.ListLogsByNumberParams) ([]db.WhatsappEventLog, error)
	ListLastLogsRepository(ctx context.Context, limit int32) ([]db.WhatsappEventLog, error)
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

// --------------------------
// DEVICES
// --------------------------
func (r *Repository) UpsertDeviceRepository(ctx context.Context, arg db.UpsertDeviceParams) (int64, error) {
	return r.queries.UpsertDevice(ctx, arg)
}

func (r *Repository) SoftDeleteDeviceRepository(ctx context.Context, number string) error {
	return r.queries.SoftDeleteDevice(ctx, number)
}

func (r *Repository) GetDeviceRepository(ctx context.Context, number string) (db.WhatsappDevice, error) {
	return r.queries.GetDevice(ctx, number)
}

func (r *Repository) GetDevicesRepository(ctx context.Context) ([]db.WhatsappDevice, error) {
	return r.queries.GetDevices(ctx)
}

// --------------------------
// WEBHOOK
// --------------------------
func (r *Repository) InsertWebhookRepository(ctx context.Context, arg db.InsertWebhookParams) error {
	return r.queries.InsertWebhook(ctx, arg)
}

func (r *Repository) ListWebhooksByDeviceRepository(ctx context.Context, device int64) ([]db.WhatsappDeviceWebhook, error) {
	return r.queries.ListWebhooksByDevice(ctx, device)
}

func (r *Repository) GetWebhookByPhraseRepository(ctx context.Context, arg db.GetWebhookByPhraseParams) (db.WhatsappDeviceWebhook, error) {
	return r.queries.GetWebhookByPhrase(ctx, arg)
}

func (r *Repository) SoftDeleteWebhookRepository(ctx context.Context, id int64) error {
	return r.queries.SoftDeleteWebhook(ctx, id)
}

// --------------------------
// EVENT LOG
// --------------------------
func (r *Repository) InsertEventLogRepository(ctx context.Context, arg db.InsertEventLogParams) error {
	return r.queries.InsertEventLog(ctx, arg)
}

func (r *Repository) ListLogsByNumberRepository(ctx context.Context, arg db.ListLogsByNumberParams) ([]db.WhatsappEventLog, error) {
	return r.queries.ListLogsByNumber(ctx, arg)
}

func (r *Repository) ListLastLogsRepository(ctx context.Context, limit int32) ([]db.WhatsappEventLog, error) {
	return r.queries.ListLastLogs(ctx, limit)
}
