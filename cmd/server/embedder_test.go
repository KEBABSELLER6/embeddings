package main

import (
	"net/http"
	"testing"

	"github.com/KEBABSELLER6/embeddings/internal/config"
	"github.com/KEBABSELLER6/embeddings/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewEmbedder(t *testing.T) {
	t.Run("ollama provider returns an ollama service", func(t *testing.T) {
		cfg := &config.Config{
			Provider: config.ProviderOllama,
			Ollama:   &config.OllamaConfig{Model: "test-model", Endpoint: "http://localhost:11434/api/embed"},
		}

		got, err := NewEmbedder(cfg, &http.Client{})

		require.NoError(t, err)
		assert.IsType(t, &service.OllamaService{}, got)
	})

	t.Run("unknown provider is an error", func(t *testing.T) {
		cfg := &config.Config{Provider: config.Provider("nope")}

		got, err := NewEmbedder(cfg, &http.Client{})

		require.Error(t, err)
		assert.ErrorContains(t, err, `unsupported provider "nope"`)
		assert.Nil(t, got)
	})

	t.Run("empty provider is an error", func(t *testing.T) {
		got, err := NewEmbedder(&config.Config{}, &http.Client{})

		require.Error(t, err)
		assert.Nil(t, got)
	})
}
