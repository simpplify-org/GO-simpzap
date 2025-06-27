package token

import (
	"time"
)

type Maker interface {
	CreateToken(userID, username, tenantID string, accessKey int64, duration time.Duration, accessID int64, document string, userOrgId int64, userEmail string) (string, error)
	CreateTokenProvider(cpnj, companyName, email string) (string, error)
	VerifyToken(token string) (*Payload, error)
	VerifyTokenProvider(token string) (*PayloadProvider, error)
}
