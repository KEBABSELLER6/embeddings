package grpchandler

import (
	"context"

	embeddingsv1 "github.com/KEBABSELLER6/embeddings/gen/embeddings/v1"
	"github.com/KEBABSELLER6/embeddings/internal/common"
)

type EmbeddingHandler struct {
	embeddingsv1.UnimplementedEmbeddingServiceServer
	service   common.EmbeddingService
	eh        *ErrorHandler
	maxInputs int
}

func NewEmbeddingHandler(service common.EmbeddingService, eh *ErrorHandler, maxInputs int) *EmbeddingHandler {
	return &EmbeddingHandler{service: service, eh: eh, maxInputs: maxInputs}
}

func (h *EmbeddingHandler) Embed(ctx context.Context, req *embeddingsv1.EmbedRequest) (*embeddingsv1.EmbedResponse, error) {
	if len(req.Input) == 0 || len(req.Input) > h.maxInputs {
		return nil, h.eh.HandleError(ctx, common.ErrInvalidInput)
	}

	resp, err := h.service.Embed(ctx, req.Input)
	if err != nil {
		return nil, h.eh.HandleError(ctx, err)
	}

	out := make([]*embeddingsv1.Embedding, len(resp.Embeddings))
	for i, v := range resp.Embeddings {
		out[i] = &embeddingsv1.Embedding{Values: v}
	}
	return &embeddingsv1.EmbedResponse{Model: resp.Model, Embeddings: out}, nil
}
