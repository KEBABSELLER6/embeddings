package main

import (
	"fmt"
	"net/http"

	"github.com/KEBABSELLER6/embeddings/internal/common"
	"github.com/KEBABSELLER6/embeddings/internal/config"
	"github.com/KEBABSELLER6/embeddings/internal/service"
)

func NewEmbedder(cfg *config.Config, cli *http.Client) (common.EmbeddingService, error) {
	switch cfg.Provider {
	case config.ProviderOllama:
		if cfg.Ollama == nil {
			return nil, fmt.Errorf("provider %q selected but Ollama config is missing", cfg.Provider)
		}

		return service.NewOllamaService(cli, *cfg.Ollama), nil
	default:
		return nil, fmt.Errorf("unsupported provider %q", cfg.Provider)
	}
}
