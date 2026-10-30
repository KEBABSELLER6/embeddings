package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"time"
)

type Provider string
type LogFormat string

const ProviderOllama Provider = "ollama"
const ProviderOpenAI Provider = "openai"
const LogFormatJSON LogFormat = "json"
const LogFormatText LogFormat = "text"

type Config struct {
	Port           string
	MaxBodySize    int64
	Provider       Provider
	Ollama         *OllamaConfig
	OpenAI         *OpenAIConfig
	Log            LogConfig
	MaxInputs      int
	Timeouts       TimeoutConfig
	GRPCPort       string
	MetricsPort    string
	BatchSize      int
	GRPCReflection bool
	APIKeyAuth     APIKeyAuth
}

type OllamaConfig struct {
	Model    string
	Endpoint string
}

type APIKeyAuth struct {
	APIKey string
}

func (a APIKeyAuth) Enabled() bool { return a.APIKey != "" }

type OpenAIConfig struct {
	APIKey  string
	Model   string
	BaseURL string
}

type LogConfig struct {
	Level  slog.Level
	Format LogFormat
}

type TimeoutConfig struct {
	Upstream, Shutdown, Drain time.Duration
}

func positiveDurationEnv(key string, def time.Duration) (time.Duration, error) {
	parsed, err := durationEnv(key, def)
	if err != nil {
		return 0, err
	}

	if parsed <= 0 {
		return 0, fmt.Errorf("%s must be positive duration", key)
	}

	return parsed, nil
}

func durationEnv(key string, def time.Duration) (time.Duration, error) {
	t := os.Getenv(key)
	if t == "" {
		return def, nil
	}

	parsed, err := time.ParseDuration(t)
	if err != nil {
		return 0, fmt.Errorf("failed to parse %s: %w", key, err)
	}

	return parsed, nil
}

func positiveIntEnv(key string, def int64) (int64, error) {
	parsed, err := intEnv(key, def)
	if err != nil {
		return 0, err
	}

	if parsed <= 0 {
		return 0, fmt.Errorf("%s must be positive", key)
	}

	return parsed, nil
}

func intEnv(key string, def int64) (int64, error) {
	i := os.Getenv(key)
	if i == "" {
		return def, nil
	}

	parsed, err := strconv.ParseInt(i, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse %s: %w", key, err)
	}

	return parsed, nil
}

func portEnv(key string, def string) (string, error) {
	port := os.Getenv(key)
	if port == "" {
		return def, nil
	}

	p, err := strconv.ParseInt(port, 10, 64)
	if err != nil {
		return "", fmt.Errorf("failed to parse %s: %w", key, err)
	}

	if p < 1 || p > 65535 {
		return "", fmt.Errorf("%s must be between 1-65535", key)
	}

	return strconv.Itoa(int(p)), nil
}

func urlEnv(key string, prvd Provider) (string, error) {
	endpoint := os.Getenv(key)
	if endpoint == "" {
		return "", fmt.Errorf("missing %s for %s provider", key, string(prvd))
	}
	u, err := url.Parse(endpoint)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("%s must be an absolute http(s) URL, got %q", key, endpoint)
	}
	return endpoint, nil
}

func Configure() (*Config, error) {
	port, err := portEnv("PORT", "8080")
	if err != nil {
		return nil, err
	}
	grpcPort, err := portEnv("GRPC_PORT", "50051")
	if err != nil {
		return nil, err
	}
	metricsPort, err := portEnv("METRICS_PORT", "9090")
	if err != nil {
		return nil, err
	}
	if grpcPort == port || grpcPort == metricsPort || metricsPort == port {
		return nil, fmt.Errorf("PORT, GRPC_PORT, METRICS_PORT must be unique")
	}

	maxBodySize, err := positiveIntEnv("MAX_BODY_SIZE", 512000)
	if err != nil {
		return nil, err
	}
	maxInputs, err := positiveIntEnv("MAX_INPUTS", 2048)
	if err != nil {
		return nil, err
	}
	upstreamTimeout, err := positiveDurationEnv("UPSTREAM_TIMEOUT", 30*time.Second)
	if err != nil {
		return nil, err
	}
	shutdownTimeout, err := positiveDurationEnv("SHUTDOWN_TIMEOUT", 35*time.Second)
	if err != nil {
		return nil, err
	}
	drainDelay, err := durationEnv("DRAIN_DELAY", 0)
	if err != nil {
		return nil, err
	}
	if drainDelay < 0 {
		return nil, fmt.Errorf("%s must be not negative duration", "DRAIN_DELAY")
	}
	if drainDelay+upstreamTimeout >= shutdownTimeout {
		return nil, fmt.Errorf("SHUTDOWN_TIMEOUT must be larger than DRAIN_DELAY + UPSTREAM_TIMEOUT")
	}
	batchSize, err := intEnv("BATCH_SIZE", 0)
	if err != nil {
		return nil, err
	}
	if batchSize < 0 {
		return nil, fmt.Errorf("%s must not be negative", "BATCH_SIZE")
	}
	refl := os.Getenv("GRPC_REFLECTION")
	var grpcRef bool
	if refl == "" {
		grpcRef = false
	} else {
		grpcRef, err = strconv.ParseBool(refl)
		if err != nil {
			return nil, fmt.Errorf("failed to parse GRPC_REFLECTION %w", err)
		}
	}

	prvdr := os.Getenv("PROVIDER")
	if prvdr == "" {
		return nil, errors.New("failed to parse PROVIDER")
	}

	var (
		provider     Provider
		ollamaConfig *OllamaConfig
		openaiConfig *OpenAIConfig
	)

	switch prvdr {
	case string(ProviderOllama):
		provider = ProviderOllama

		endpoint, err := urlEnv("OLLAMA_ENDPOINT_URL", ProviderOllama)
		if err != nil {
			return nil, err
		}

		model := os.Getenv("OLLAMA_MODEL")
		if model == "" {
			return nil, errors.New("missing OLLAMA_MODEL for ollama provider")
		}
		ollamaConfig = &OllamaConfig{
			Model:    model,
			Endpoint: endpoint,
		}
	case string(ProviderOpenAI):
		provider = ProviderOpenAI

		apiKey := os.Getenv("OPENAI_API_KEY")
		if apiKey == "" {
			return nil, errors.New("missing OPENAI_API_KEY for openai provider")
		}

		model := os.Getenv("OPENAI_MODEL")
		if model == "" {
			return nil, errors.New("missing OPENAI_MODEL for openai provider")
		}

		baseURL, err := urlEnv("OPENAI_BASE_URL", ProviderOpenAI)
		if err != nil {
			return nil, err
		}

		openaiConfig = &OpenAIConfig{
			Model:   model,
			APIKey:  apiKey,
			BaseURL: baseURL,
		}
	default:
		return nil, fmt.Errorf("unsupported PROVIDER %q", prvdr)
	}

	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "info"
	}
	var lvl slog.Level
	err = lvl.UnmarshalText([]byte(logLevel))
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

	apiKey := os.Getenv("API_KEY")

	config := &Config{
		Port:           port,
		MaxBodySize:    maxBodySize,
		Provider:       provider,
		Ollama:         ollamaConfig,
		Log:            LogConfig{Format: format, Level: lvl},
		MaxInputs:      int(maxInputs),
		Timeouts:       TimeoutConfig{Upstream: upstreamTimeout, Shutdown: shutdownTimeout, Drain: drainDelay},
		GRPCPort:       grpcPort,
		MetricsPort:    metricsPort,
		BatchSize:      int(batchSize),
		OpenAI:         openaiConfig,
		GRPCReflection: grpcRef,
		APIKeyAuth:     APIKeyAuth{APIKey: apiKey},
	}

	return config, nil
}
