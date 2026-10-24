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
	eh      *ErrorHandler
}

func NewEmbeddingHandler(service EmbeddingService, eh *ErrorHandler) *EmbeddingHandler {
	return &EmbeddingHandler{service: service, eh: eh}
}

type EmbeddingRequest struct {
	Input []string `json:"input"`
}

func (h *EmbeddingHandler) HandleEmbeddingRequest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var b EmbeddingRequest
	err := json.UnmarshalRead(r.Body, &b)
	if err != nil {
		h.eh.HandleError(w, r, fmt.Errorf("%w: %w", common.ErrInvalidInput, err))
		return
	}

	if len(b.Input) == 0 || len(b.Input) > 2048 {
		h.eh.HandleError(w, r, common.ErrInvalidInput)
		return
	}

	embeddings, err := h.service.Embed(r.Context(), b.Input)
	if err != nil {
		h.eh.HandleError(w, r, err)
		return
	}

	out, err := json.Marshal(embeddings)
	if err != nil {
		h.eh.HandleError(w, r, fmt.Errorf("internal error: %v", err))
		return
	}

	w.Write(out)
}
