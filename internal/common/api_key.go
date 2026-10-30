package common

type APIKeyService interface {
	Authenticate(incomming string) bool
}
