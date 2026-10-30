package auth

import (
	"crypto/sha256"
	"crypto/subtle"
)

type APIKeyService struct {
	apiKey string
}

func NewAPIKeyService(APIKey string) *APIKeyService {
	return &APIKeyService{apiKey: APIKey}
}

func (s *APIKeyService) Authenticate(incoming string) bool {
	a := sha256.Sum256([]byte(incoming))
	b := sha256.Sum256([]byte(s.apiKey))
	return subtle.ConstantTimeCompare(a[:], b[:]) == 1
}
