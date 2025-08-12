package app

import "go.mongodb.org/mongo-driver/bson/primitive"

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
