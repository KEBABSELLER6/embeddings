package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
)

type Provider string
type LogFormat string

const ProviderOllama Provider = "ollama"
const LogFormatJSON LogFormat = "json"
const LogFormatText LogFormat = "text"

type Config struct {
	Port        string
	MaxBodySize int64
	Provider    Provider
	Ollama      *OllamaConfig
	Log         LogConfig
}

type OllamaConfig struct {
	Model    string
	Endpoint string
}

type LogConfig struct {
	Level  slog.Level
	Format LogFormat
}

func Configure() (*Config, error) {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	var maxBodySize int64

	maxSize := os.Getenv("MAX_BODY_SIZE")
	if maxSize != "" {
		m, err := strconv.ParseInt(maxSize, 10, 64)
		if err != nil {
			return nil, errors.New("failed to parse MAX_BODY_SIZE")
		}
		if m <= 0 {
			return nil, errors.New("MAX_BODY_SIZE must be positive")
		}

		maxBodySize = m
	} else {
		maxBodySize = 512000
	}

	prvdr := os.Getenv("PROVIDER")
	if prvdr == "" {
		return nil, errors.New("failed to parse PROVIDER")
	}

	var (
		provider     Provider
		ollamaConfig *OllamaConfig
	)

	switch prvdr {
	case string(ProviderOllama):
		provider = ProviderOllama

		endpoint := os.Getenv("OLLAMA_ENDPOINT_URL")
		if endpoint == "" {
			return nil, errors.New("missing OLLAMA_ENDPOINT_URL for ollama provider")
		}
		model := os.Getenv("OLLAMA_MODEL")
		if model == "" {
			return nil, errors.New("missing OLLAMA_MODEL for ollama provider")
		}
		ollamaConfig = &OllamaConfig{
			Model:    model,
			Endpoint: endpoint,
		}
	default:
		return nil, fmt.Errorf("unsupported PROVIDER %q", prvdr)
	}

	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "info"
	}
	var lvl slog.Level
	err := lvl.UnmarshalText([]byte(logLevel))
	if err != nil {
		return nil, fmt.Errorf("unsupported LOG_LEVEL: %q", logLevel)
	}

	logFormat := os.Getenv("LOG_FORMAT")
	var format LogFormat
	switch logFormat {
	case string(LogFormatJSON):
		format = LogFormatJSON
	case "", string(LogFormatText):
		format = LogFormatText
	default:
		return nil, fmt.Errorf("unsupported LOG_FORMAT: %q", logFormat)
	}

	config := &Config{
		Port:        port,
		MaxBodySize: maxBodySize,
		Provider:    provider,
		Ollama:      ollamaConfig,
		Log:         LogConfig{Format: format, Level: lvl},
	}

	return config, nil
}
