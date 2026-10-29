package common

import (
	"context"
)

type EmbeddingResponse struct {
	Model      string      `json:"model"`
	Embeddings [][]float32 `json:"embeddings"`
}

type EmbeddingService interface {
	Embed(ctx context.Context, input []string) (*EmbeddingResponse, error)
}
