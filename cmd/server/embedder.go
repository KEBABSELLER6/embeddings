package main

import (
	"fmt"
	"net/http"

	"github.com/KEBABSELLER6/embeddings/internal/config"
	"github.com/KEBABSELLER6/embeddings/internal/handler"
	"github.com/KEBABSELLER6/embeddings/internal/service"
)

func NewEmbedder(cfg *config.Config, cli *http.Client) (handler.EmbeddingService, error) {
	switch cfg.Provider {
	case config.ProviderOllama:
		return service.NewOllamaService(cli, *cfg.Ollama), nil
	default:
		return nil, fmt.Errorf("unsupported provider %q", cfg.Provider)
	}
}
