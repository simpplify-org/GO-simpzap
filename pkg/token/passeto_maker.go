package token

import (
	"fmt"
	"time"

	"github.com/o1egl/paseto"
	"golang.org/x/crypto/chacha20poly1305"
)

type PasetoMaker struct {
	paseto      *paseto.V2
	symetricKey []byte
}

func NewPasetoMaker(symetricKey string) (Maker, error) {
	if len(symetricKey) != chacha20poly1305.KeySize {
		return nil, fmt.Errorf("invalid key size: must be exactly %d characacteres", chacha20poly1305.KeySize)
	}

	maker := &PasetoMaker{
		paseto:      paseto.NewV2(),
		symetricKey: []byte(symetricKey),
	}

	return maker, nil
}

func (maker *PasetoMaker) CreateToken(userID, username, tenantID string, accessKey int64, duration time.Duration, accessID int64, document string, userOrgId int64, userEmail string) (string, error) {
	payload, err := NewPayload(userID, username, tenantID, accessKey, duration, accessID, document, userOrgId, userEmail)
	if err != nil {
		return "", err
	}

	return maker.paseto.Encrypt(maker.symetricKey, payload, nil)
}

func (maker *PasetoMaker) CreateTokenProvider(cpnj, companyName, email string) (string, error) {
	payload := NewPayloadProvider(cpnj, companyName, email)

	return maker.paseto.Encrypt(maker.symetricKey, payload, nil)
}

func (maker *PasetoMaker) VerifyToken(token string) (*Payload, error) {
	payload := &Payload{}

	err := maker.paseto.Decrypt(token, maker.symetricKey, payload, nil)
	if err != nil {
		return nil, ErrInvalidToken
	}

	err = payload.valid()
	if err != nil {
		return nil, err
	}

	return payload, nil
}

func (maker *PasetoMaker) VerifyTokenProvider(token string) (*PayloadProvider, error) {
	payload := &PayloadProvider{}

	err := maker.paseto.Decrypt(token, maker.symetricKey, payload, nil)
	if err != nil {
		return nil, ErrInvalidToken
	}

	return payload, nil
}
