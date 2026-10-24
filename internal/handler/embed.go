package handler

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"net/http"

	"github.com/KEBABSELLER6/embeddings/internal/common"
)

type EmbeddingService interface {
	Embed(ctx context.Context, input []string) (*common.EmbeddingResponse, error)
}

type EmbeddingHandler struct {
	service EmbeddingService
}

func NewEmbeddingHandler(service EmbeddingService) *EmbeddingHandler {
	return &EmbeddingHandler{service: service}
}

type EmbeddingRequest struct {
	Input []string `json:"input"`
}

func (h *EmbeddingHandler) HandleEmbeddingRequest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var b EmbeddingRequest
	err := json.UnmarshalRead(r.Body, &b)
	if err != nil {
		HandleError(w, r, fmt.Errorf("%w: %w", common.ErrInvalidInput, err))
		return
	}

	if len(b.Input) == 0 || len(b.Input) > 2048 {
		HandleError(w, r, common.ErrInvalidInput)
		return
	}

	embeddings, err := h.service.Embed(r.Context(), b.Input)
	if err != nil {
		HandleError(w, r, err)
		return
	}

	out, err := json.Marshal(embeddings)
	if err != nil {
		HandleError(w, r, fmt.Errorf("internal error: %v", err))
		return
	}

	w.Write(out)
}
