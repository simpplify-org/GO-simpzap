package token

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var ErrExpiredToken = errors.New("token has expired")
var ErrInvalidToken = errors.New("token is invalid")

type Payload struct {
	ID             uuid.UUID `json:"id"`
	UserNickname   string    `json:"user_nickname"`
	UserID         string    `json:"user_id"`
	AccessKey      int64     `json:"access_key"`
	AccessID       int64     `json:"access_id"`
	TenantID       string    `json:"tenant_id"`
	IssuedAt       time.Time `json:"issued_at"`
	ExpiredAt      time.Time `json:"expired_at"`
	Document       string    `json:"document"`
	UserOrgId      int64     `json:"user_org_id"`
	UserEmail      string    `json:"user_email"`
	UserName       string    `json:"user_name"`
	OrganizationID int64     `json:"organization_id"`
}

type PayloadProvider struct {
	Cnpj        string `json:"cnpj"`
	CompanyName string `json:"company_name"`
	Email       string `json:"email"`
}

func NewPayload(userID, username, tenantID string, accessKey int64, duration time.Duration, accessID int64, document string, userOrgId int64, userEmail string) (*Payload, error) {
	tokenID, err := uuid.NewRandom()
	if err != nil {
		return nil, err
	}

	payload := &Payload{
		ID:           tokenID,
		UserNickname: username,
		UserID:       userID,
		AccessKey:    accessKey,
		AccessID:     accessID,
		TenantID:     tenantID,
		IssuedAt:     time.Now(),
		ExpiredAt:    time.Now().Add(duration),
		Document:     document,
		UserOrgId:    userOrgId,
		UserEmail:    userEmail,
		UserName:     "",
	}

	return payload, nil
}

func NewPayloadProvider(cpnj, companyName, email string) *PayloadProvider {
	payload := &PayloadProvider{
		Cnpj:        cpnj,
		CompanyName: companyName,
		Email:       email,
	}

	return payload
}

func (payload *Payload) valid() error {
	if time.Now().After(payload.ExpiredAt) {
		return ErrExpiredToken
	}
	return nil
}
