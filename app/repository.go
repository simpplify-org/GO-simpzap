package app

import (
	"context"
	"errors"
	"fmt"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"log"
	"time"
)

type DeviceRepository struct {
	Collection *mongo.Collection
}

func NewDeviceRepository(db *mongo.Database) *DeviceRepository {
	repo := &DeviceRepository{Collection: db.Collection("devices")}
	indexModel := mongo.IndexModel{
		Keys: bson.D{
			{Key: "tenant_id", Value: 1},
			{Key: "number", Value: 1},
		},
		Options: options.Index().SetUnique(true),
	}
	_, err := repo.Collection.Indexes().CreateOne(context.Background(), indexModel)
	if err != nil {
		log.Fatalf("Erro ao criar índice único: %v", err)
	}
	return repo
}

func (r *DeviceRepository) Create(ctx context.Context, tenantID, number string) (*Device, error) {
	device := &Device{
		TenantID:  tenantID,
		Number:    number,
		CreatedAt: time.Now().Unix(),
	}
	res, err := r.Collection.InsertOne(ctx, device)
	if err != nil {
		return nil, err
	}
	device.ID = res.InsertedID.(primitive.ObjectID)
	return device, nil
}

func (r *DeviceRepository) GetByID(ctx context.Context, id string) (*Device, error) {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, fmt.Errorf("id inválido: %w", err)
	}

	var device Device
	err = r.Collection.FindOne(ctx, bson.M{"_id": objID}).Decode(&device)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, fmt.Errorf("device não encontrado")
		}
		return nil, err
	}
	return &device, nil
}

func (r *DeviceRepository) SaveSession(ctx context.Context, deviceID string, sessionData []byte) error {
	objID, err := primitive.ObjectIDFromHex(deviceID)
	if err != nil {
		return fmt.Errorf("deviceID inválido: %w", err)
	}
	filter := bson.M{"_id": objID}
	update := bson.M{"$set": bson.M{"session_db": sessionData}}

	_, err = r.Collection.UpdateOne(ctx, filter, update)
	return err
}

func (r *DeviceRepository) GetSessionByDeviceID(ctx context.Context, deviceID string) ([]byte, error) {
	objID, err := primitive.ObjectIDFromHex(deviceID)
	if err != nil {
		return nil, fmt.Errorf("deviceID inválido: %w", err)
	}
	filter := bson.M{"_id": objID}

	var device Device
	err = r.Collection.FindOne(ctx, filter).Decode(&device)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, fmt.Errorf("sessão não encontrada para deviceID")
		}
		return nil, err
	}
	return device.SessionDB, nil
}
