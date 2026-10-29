package handler

import (
	"encoding/json/v2"
	"fmt"
	"net/http"

	"github.com/KEBABSELLER6/embeddings/internal/common"
)

type EmbeddingHandler struct {
	service   common.EmbeddingService
	eh        *ErrorHandler
	maxInputs int
}

func NewEmbeddingHandler(service common.EmbeddingService, eh *ErrorHandler, maxInputs int) *EmbeddingHandler {
	return &EmbeddingHandler{service: service, eh: eh, maxInputs: maxInputs}
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

	if len(b.Input) == 0 || len(b.Input) > h.maxInputs {
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
