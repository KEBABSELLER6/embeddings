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
	t.Run("ollama provider is wrapped in a chunker", func(t *testing.T) {
		cfg := &config.Config{
			Provider: config.ProviderOllama,
			Ollama:   &config.OllamaConfig{Model: "test-model", Endpoint: "http://localhost:11434/api/embed"},
		}

		got, err := NewEmbedder(cfg, &http.Client{}, 0)

		require.NoError(t, err)
		assert.IsType(t, &service.Chunker{}, got)
	})

	t.Run("ollama provider without ollama config is an error", func(t *testing.T) {
		cfg := &config.Config{Provider: config.ProviderOllama}

		got, err := NewEmbedder(cfg, &http.Client{}, 0)

		require.Error(t, err)
		assert.ErrorContains(t, err, "Ollama config is missing")
		assert.Nil(t, got)
	})

	t.Run("openai provider is wrapped in a chunker", func(t *testing.T) {
		cfg := &config.Config{
			Provider: config.ProviderOpenAI,
			OpenAI:   &config.OpenAIConfig{APIKey: "sk-test", Model: "test-model", BaseURL: "https://api.openai.com/v1"},
		}

		got, err := NewEmbedder(cfg, &http.Client{}, 0)

		require.NoError(t, err)
		assert.IsType(t, &service.Chunker{}, got)
	})

	t.Run("openai provider without openai config is an error", func(t *testing.T) {
		cfg := &config.Config{Provider: config.ProviderOpenAI}

		got, err := NewEmbedder(cfg, &http.Client{}, 0)

		require.Error(t, err)
		assert.ErrorContains(t, err, "OpenAI config is missing")
		assert.Nil(t, got)
	})

	t.Run("unknown provider is an error", func(t *testing.T) {
		cfg := &config.Config{Provider: config.Provider("nope")}

		got, err := NewEmbedder(cfg, &http.Client{}, 0)

		require.Error(t, err)
		assert.ErrorContains(t, err, `unsupported provider "nope"`)
		assert.Nil(t, got)
	})

	t.Run("empty provider is an error", func(t *testing.T) {
		got, err := NewEmbedder(&config.Config{}, &http.Client{}, 0)

		require.Error(t, err)
		assert.Nil(t, got)
	})
}
