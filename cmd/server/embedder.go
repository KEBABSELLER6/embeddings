package main

import (
	"fmt"
	"net/http"

	"github.com/KEBABSELLER6/embeddings/internal/common"
	"github.com/KEBABSELLER6/embeddings/internal/config"
	"github.com/KEBABSELLER6/embeddings/internal/service"
)

func NewEmbedder(cfg *config.Config, cli *http.Client, batchSize int) (common.EmbeddingService, error) {
	switch cfg.Provider {
	case config.ProviderOllama:
		if cfg.Ollama == nil {
			return nil, fmt.Errorf("provider %q selected but Ollama config is missing", cfg.Provider)
		}

		ollama := service.NewOllamaService(cli, *cfg.Ollama)

		return service.NewChunker(ollama, batchSize), nil
	case config.ProviderOpenAI:
		if cfg.OpenAI == nil {
			return nil, fmt.Errorf("provider %q selected but OpenAI config is missing", cfg.Provider)
		}

		openai := service.NewOpenAIService(*cfg.OpenAI, cli)

		return service.NewChunker(openai, batchSize), nil
	default:
		return nil, fmt.Errorf("unsupported provider %q", cfg.Provider)
	}
}
