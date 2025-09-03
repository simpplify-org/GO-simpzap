package app

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type SendMessageRequest struct {
	DeviceID string `json:"device_id"`
	Number   string `json:"number"`
	Message  string `json:"message"`
}

type Device struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"device_id"`
	TenantID  string             `bson:"tenant_id"`
	Number    string             `bson:"number"`
	CreatedAt int64              `bson:"created_at"`
	SessionDB []byte             `bson:"session_db,omitempty"`
	Connected bool               `bson:"connected"`
	UpdatedAt int64              `bson:"updated_at"`
}

type DeviceResponse struct {
	ID        string `json:"id"`
	TenantID  string `json:"tenant_id"`
	Number    string `json:"number"`
	Connected bool   `json:"connected"`
	CreatedAt int64  `json:"created_at"`
}

type MessageHistory struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	TenantID  string             `bson:"tenant_id" json:"tenant_id"`
	DeviceID  string             `bson:"device_id" json:"device_id"`
	Number    string             `bson:"number" json:"number"`
	Message   string             `bson:"message" json:"message"`
	Status    string             `bson:"status" json:"status"` // "sent", "failed", "delivered"
	Timestamp time.Time          `bson:"timestamp" json:"timestamp"`
}

type SendBulkMessageRequest struct {
	DeviceID string   `json:"device_id" validate:"required"`
	Numbers  []string `json:"numbers" validate:"required,dive,required"`
	Message  string   `json:"message" validate:"required"`
}
