package grpchandler

import (
	"context"

	"github.com/KEBABSELLER6/embeddings/internal/common"
	"google.golang.org/grpc/metadata"
)

type APIKeyAuthHandler struct {
	s       common.APIKeyService
	enabled bool
	eh      *ErrorHandler
}

func NewAPIKeyAuth(s common.APIKeyService, enabled bool, eh *ErrorHandler) *APIKeyAuthHandler {
	return &APIKeyAuthHandler{s: s, enabled: enabled, eh: eh}
}

func (h *APIKeyAuthHandler) APIKeyAuth(ctx context.Context) (context.Context, error) {
	if !h.enabled {
		return ctx, nil
	}

	// No metadata at all also means no key.
	md, _ := metadata.FromIncomingContext(ctx)
	values := md.Get("x-api-key")
	if len(values) == 0 || values[0] == "" {
		return nil, h.eh.HandleError(ctx, common.ErrMissingAPIKey)
	}
	apiKey := values[0]

	if h.s.Authenticate(apiKey) {
		return ctx, nil
	}

	return nil, h.eh.HandleError(ctx, common.ErrAuthFailed)
}
