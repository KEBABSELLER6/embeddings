package handler

import (
	"net/http"

	"github.com/KEBABSELLER6/embeddings/internal/common"
)

type APIKeyAuthHandler struct {
	s       common.APIKeyService
	enabled bool
	eh      *ErrorHandler
}

func NewAPIKeyAuth(s common.APIKeyService, enabled bool, eh *ErrorHandler) *APIKeyAuthHandler {
	return &APIKeyAuthHandler{s: s, enabled: enabled, eh: eh}
}

func (h *APIKeyAuthHandler) Authenticate(next http.Handler) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		if !h.enabled {
			next.ServeHTTP(rw, r)
			return
		}

		apiKey := r.Header.Get("X-API-Key")
		if apiKey == "" {
			h.eh.HandleError(rw, r, common.ErrMissingAPIKey)
			return
		}

		if !h.s.Authenticate(apiKey) {
			h.eh.HandleError(rw, r, common.ErrAuthFailed)
			return
		}

		next.ServeHTTP(rw, r)
	}
}
