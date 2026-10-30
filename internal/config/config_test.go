package config

import (
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var envKeys = []string{
	"PORT",
	"GRPC_PORT",
	"METRICS_PORT",
	"MAX_BODY_SIZE",
	"PROVIDER",
	"OLLAMA_MODEL",
	"OLLAMA_ENDPOINT_URL",
	"LOG_LEVEL",
	"LOG_FORMAT",
	"MAX_INPUTS",
	"UPSTREAM_TIMEOUT",
	"SHUTDOWN_TIMEOUT",
	"BATCH_SIZE",
}

// setEnv clears every variable Configure reads, then applies env, so a test
// never sees values from the developer's shell.
func setEnv(t *testing.T, env map[string]string) {
	t.Helper()
	for _, k := range envKeys {
		t.Setenv(k, "")
	}
	for k, v := range env {
		t.Setenv(k, v)
	}
}

func TestConfigure(t *testing.T) {
	ollamaEnv := func(overrides map[string]string) map[string]string {
		env := map[string]string{
			"PROVIDER":            "ollama",
			"OLLAMA_MODEL":        "nomic-embed-text",
			"OLLAMA_ENDPOINT_URL": "http://localhost:11434/api/embed",
		}
		for k, v := range overrides {
			env[k] = v
		}
		return env
	}

	defaultLog := LogConfig{Level: slog.LevelInfo, Format: LogFormatText}
	defaultTimeouts := TimeoutConfig{Upstream: 30 * time.Second, Shutdown: 35 * time.Second}

	// defaultOllamaConfig is what ollamaEnv(nil) produces, with log settings swapped in.
	defaultOllamaConfig := func(log LogConfig) *Config {
		return &Config{
			Port:        "8080",
			GRPCPort:    "50051",
			MetricsPort: "9090",
			MaxBodySize: 512000,
			Provider:    ProviderOllama,
			Ollama: &OllamaConfig{
				Model:    "nomic-embed-text",
				Endpoint: "http://localhost:11434/api/embed",
			},
			Log:       log,
			MaxInputs: 2048,
			Timeouts:  defaultTimeouts,
			BatchSize: 0,
		}
	}

	tests := []struct {
		name    string
		env     map[string]string
		want    *Config
		wantErr string
	}{
		{
			name: "defaults for port and body size",
			env:  ollamaEnv(nil),
			want: &Config{
				Port:        "8080",
				GRPCPort:    "50051",
				MetricsPort: "9090",
				MaxBodySize: 512000,
				Provider:    ProviderOllama,
				Ollama: &OllamaConfig{
					Model:    "nomic-embed-text",
					Endpoint: "http://localhost:11434/api/embed",
				},
				Log:       defaultLog,
				MaxInputs: 2048,
				Timeouts:  defaultTimeouts,
			},
		},
		{
			name: "custom ports and body size",
			env:  ollamaEnv(map[string]string{"PORT": "9000", "GRPC_PORT": "9001", "METRICS_PORT": "9002", "MAX_BODY_SIZE": "256000"}),
			want: &Config{
				Port:        "9000",
				GRPCPort:    "9001",
				MetricsPort: "9002",
				MaxBodySize: 256000,
				Provider:    ProviderOllama,
				Ollama: &OllamaConfig{
					Model:    "nomic-embed-text",
					Endpoint: "http://localhost:11434/api/embed",
				},
				Log:       defaultLog,
				MaxInputs: 2048,
				Timeouts:  defaultTimeouts,
			},
		},
		{
			name: "ports at the bounds of the valid range",
			env:  ollamaEnv(map[string]string{"PORT": "1", "GRPC_PORT": "65535", "METRICS_PORT": "2"}),
			want: func() *Config {
				c := defaultOllamaConfig(defaultLog)
				c.Port = "1"
				c.GRPCPort = "65535"
				c.MetricsPort = "2"
				return c
			}(),
		},
		{
			name:    "port is not a number",
			env:     ollamaEnv(map[string]string{"PORT": "abc"}),
			wantErr: "failed to parse PORT",
		},
		{
			name:    "port is a decimal",
			env:     ollamaEnv(map[string]string{"PORT": "80.5"}),
			wantErr: "failed to parse PORT",
		},
		{
			name:    "port is zero",
			env:     ollamaEnv(map[string]string{"PORT": "0"}),
			wantErr: "PORT must be between 1-65535",
		},
		{
			name:    "port is negative",
			env:     ollamaEnv(map[string]string{"PORT": "-1"}),
			wantErr: "PORT must be between 1-65535",
		},
		{
			name:    "port is above the valid range",
			env:     ollamaEnv(map[string]string{"PORT": "65536"}),
			wantErr: "PORT must be between 1-65535",
		},
		{
			name:    "grpc port is not a number",
			env:     ollamaEnv(map[string]string{"GRPC_PORT": "abc"}),
			wantErr: "failed to parse GRPC_PORT",
		},
		{
			name:    "grpc port is zero",
			env:     ollamaEnv(map[string]string{"GRPC_PORT": "0"}),
			wantErr: "GRPC_PORT must be between 1-65535",
		},
		{
			name:    "grpc port is above the valid range",
			env:     ollamaEnv(map[string]string{"GRPC_PORT": "65536"}),
			wantErr: "GRPC_PORT must be between 1-65535",
		},
		{
			name:    "metrics port is not a number",
			env:     ollamaEnv(map[string]string{"METRICS_PORT": "abc"}),
			wantErr: "failed to parse METRICS_PORT",
		},
		{
			name:    "metrics port is zero",
			env:     ollamaEnv(map[string]string{"METRICS_PORT": "0"}),
			wantErr: "METRICS_PORT must be between 1-65535",
		},
		{
			name:    "metrics port is above the valid range",
			env:     ollamaEnv(map[string]string{"METRICS_PORT": "65536"}),
			wantErr: "METRICS_PORT must be between 1-65535",
		},
		{
			name:    "metrics port equals port",
			env:     ollamaEnv(map[string]string{"PORT": "9000", "METRICS_PORT": "9000"}),
			wantErr: "PORT, GRPC_PORT, METRICS_PORT must be unique",
		},
		{
			name:    "metrics port equals grpc port",
			env:     ollamaEnv(map[string]string{"GRPC_PORT": "9000", "METRICS_PORT": "9000"}),
			wantErr: "PORT, GRPC_PORT, METRICS_PORT must be unique",
		},
		{
			name:    "metrics port equals the default port",
			env:     ollamaEnv(map[string]string{"METRICS_PORT": "8080"}),
			wantErr: "PORT, GRPC_PORT, METRICS_PORT must be unique",
		},
		{
			name:    "port equals the default metrics port",
			env:     ollamaEnv(map[string]string{"PORT": "9090"}),
			wantErr: "PORT, GRPC_PORT, METRICS_PORT must be unique",
		},
		{
			name:    "grpc port equals port",
			env:     ollamaEnv(map[string]string{"PORT": "9000", "GRPC_PORT": "9000"}),
			wantErr: "PORT, GRPC_PORT, METRICS_PORT must be unique",
		},
		{
			name:    "grpc port equals the default port",
			env:     ollamaEnv(map[string]string{"GRPC_PORT": "8080"}),
			wantErr: "PORT, GRPC_PORT, METRICS_PORT must be unique",
		},
		{
			name:    "port equals the default grpc port",
			env:     ollamaEnv(map[string]string{"PORT": "50051"}),
			wantErr: "PORT, GRPC_PORT, METRICS_PORT must be unique",
		},
		{
			name: "custom ollama model and endpoint",
			env: ollamaEnv(map[string]string{
				"OLLAMA_MODEL":        "mxbai-embed-large",
				"OLLAMA_ENDPOINT_URL": "http://ollama:11434/api/embed",
			}),
			want: &Config{
				Port:        "8080",
				GRPCPort:    "50051",
				MetricsPort: "9090",
				MaxBodySize: 512000,
				Provider:    ProviderOllama,
				Ollama: &OllamaConfig{
					Model:    "mxbai-embed-large",
					Endpoint: "http://ollama:11434/api/embed",
				},
				Log:       defaultLog,
				MaxInputs: 2048,
				Timeouts:  defaultTimeouts,
			},
		},
		{
			name: "debug log level",
			env:  ollamaEnv(map[string]string{"LOG_LEVEL": "debug"}),
			want: defaultOllamaConfig(LogConfig{Level: slog.LevelDebug, Format: LogFormatText}),
		},
		{
			name: "log level is case insensitive",
			env:  ollamaEnv(map[string]string{"LOG_LEVEL": "DEBUG"}),
			want: defaultOllamaConfig(LogConfig{Level: slog.LevelDebug, Format: LogFormatText}),
		},
		{
			name: "warn log level",
			env:  ollamaEnv(map[string]string{"LOG_LEVEL": "warn"}),
			want: defaultOllamaConfig(LogConfig{Level: slog.LevelWarn, Format: LogFormatText}),
		},
		{
			name: "error log level",
			env:  ollamaEnv(map[string]string{"LOG_LEVEL": "error"}),
			want: defaultOllamaConfig(LogConfig{Level: slog.LevelError, Format: LogFormatText}),
		},
		{
			name: "json log format",
			env:  ollamaEnv(map[string]string{"LOG_FORMAT": "json"}),
			want: defaultOllamaConfig(LogConfig{Level: slog.LevelInfo, Format: LogFormatJSON}),
		},
		{
			name: "explicit text log format",
			env:  ollamaEnv(map[string]string{"LOG_FORMAT": "text"}),
			want: defaultOllamaConfig(defaultLog),
		},
		{
			name:    "log level is unknown",
			env:     ollamaEnv(map[string]string{"LOG_LEVEL": "verbose"}),
			wantErr: `unsupported LOG_LEVEL: "verbose"`,
		},
		{
			name:    "log format is unknown",
			env:     ollamaEnv(map[string]string{"LOG_FORMAT": "xml"}),
			wantErr: `unsupported LOG_FORMAT: "xml"`,
		},
		{
			name:    "log format is case sensitive",
			env:     ollamaEnv(map[string]string{"LOG_FORMAT": "JSON"}),
			wantErr: `unsupported LOG_FORMAT: "JSON"`,
		},
		{
			name: "custom limits and timeouts",
			env: ollamaEnv(map[string]string{
				"MAX_INPUTS":       "16",
				"UPSTREAM_TIMEOUT": "1m30s",
				"SHUTDOWN_TIMEOUT": "2m",
			}),
			want: func() *Config {
				c := defaultOllamaConfig(defaultLog)
				c.MaxInputs = 16
				c.Timeouts = TimeoutConfig{Upstream: 90 * time.Second, Shutdown: 2 * time.Minute}
				return c
			}(),
		},
		{
			name: "batch size is set",
			env:  ollamaEnv(map[string]string{"BATCH_SIZE": "64"}),
			want: func() *Config {
				c := defaultOllamaConfig(defaultLog)
				c.BatchSize = 64
				return c
			}(),
		},
		{
			name:    "batch size is not a number",
			env:     ollamaEnv(map[string]string{"BATCH_SIZE": "lots"}),
			wantErr: "failed to parse BATCH_SIZE",
		},
		{
			name:    "batch size is negative",
			env:     ollamaEnv(map[string]string{"BATCH_SIZE": "-1"}),
			wantErr: "BATCH_SIZE must not be negative",
		},
		{
			name:    "max inputs is not a number",
			env:     ollamaEnv(map[string]string{"MAX_INPUTS": "lots"}),
			wantErr: "failed to parse MAX_INPUTS",
		},
		{
			name:    "max inputs is zero",
			env:     ollamaEnv(map[string]string{"MAX_INPUTS": "0"}),
			wantErr: "MAX_INPUTS must be positive",
		},
		{
			name:    "upstream timeout has no unit",
			env:     ollamaEnv(map[string]string{"UPSTREAM_TIMEOUT": "30"}),
			wantErr: "failed to parse UPSTREAM_TIMEOUT",
		},
		{
			name:    "upstream timeout is zero",
			env:     ollamaEnv(map[string]string{"UPSTREAM_TIMEOUT": "0s"}),
			wantErr: "UPSTREAM_TIMEOUT must be positive",
		},
		{
			name:    "upstream timeout is negative",
			env:     ollamaEnv(map[string]string{"UPSTREAM_TIMEOUT": "-5s"}),
			wantErr: "UPSTREAM_TIMEOUT must be positive",
		},
		{
			name:    "shutdown timeout is not a duration",
			env:     ollamaEnv(map[string]string{"SHUTDOWN_TIMEOUT": "soon"}),
			wantErr: "failed to parse SHUTDOWN_TIMEOUT",
		},
		{
			name:    "shutdown timeout is zero",
			env:     ollamaEnv(map[string]string{"SHUTDOWN_TIMEOUT": "0s"}),
			wantErr: "SHUTDOWN_TIMEOUT must be positive",
		},
		{
			name:    "shutdown timeout below upstream timeout",
			env:     ollamaEnv(map[string]string{"SHUTDOWN_TIMEOUT": "10s"}),
			wantErr: "SHUTDOWN_TIMEOUT must be larger than UPSTREAM_TIMEOUT",
		},
		{
			name:    "shutdown timeout equal to upstream timeout",
			env:     ollamaEnv(map[string]string{"UPSTREAM_TIMEOUT": "30s", "SHUTDOWN_TIMEOUT": "30s"}),
			wantErr: "SHUTDOWN_TIMEOUT must be larger than UPSTREAM_TIMEOUT",
		},
		{
			name:    "raising only the upstream timeout past the default shutdown timeout",
			env:     ollamaEnv(map[string]string{"UPSTREAM_TIMEOUT": "40s"}),
			wantErr: "SHUTDOWN_TIMEOUT must be larger than UPSTREAM_TIMEOUT",
		},
		{
			name:    "body size is not a number",
			env:     ollamaEnv(map[string]string{"MAX_BODY_SIZE": "lots"}),
			wantErr: "failed to parse MAX_BODY_SIZE",
		},
		{
			name:    "body size is a decimal",
			env:     ollamaEnv(map[string]string{"MAX_BODY_SIZE": "1.5"}),
			wantErr: "failed to parse MAX_BODY_SIZE",
		},
		{
			name:    "body size is zero",
			env:     ollamaEnv(map[string]string{"MAX_BODY_SIZE": "0"}),
			wantErr: "MAX_BODY_SIZE must be positive",
		},
		{
			name:    "body size is negative",
			env:     ollamaEnv(map[string]string{"MAX_BODY_SIZE": "-1"}),
			wantErr: "MAX_BODY_SIZE must be positive",
		},
		{
			name:    "provider is missing",
			env:     map[string]string{},
			wantErr: "PROVIDER",
		},
		{
			name:    "provider is unknown",
			env:     ollamaEnv(map[string]string{"PROVIDER": "nope"}),
			wantErr: `unsupported PROVIDER "nope"`,
		},
		{
			name:    "provider is case sensitive",
			env:     ollamaEnv(map[string]string{"PROVIDER": "Ollama"}),
			wantErr: `unsupported PROVIDER "Ollama"`,
		},
		{
			name:    "ollama endpoint is missing",
			env:     ollamaEnv(map[string]string{"OLLAMA_ENDPOINT_URL": ""}),
			wantErr: "missing OLLAMA_ENDPOINT_URL",
		},
		{
			name:    "ollama endpoint without scheme",
			env:     ollamaEnv(map[string]string{"OLLAMA_ENDPOINT_URL": "localhost:11434/api/embed"}),
			wantErr: `OLLAMA_ENDPOINT_URL must be an absolute http(s) URL, got "localhost:11434/api/embed"`,
		},
		{
			name:    "ollama endpoint is a bare word",
			env:     ollamaEnv(map[string]string{"OLLAMA_ENDPOINT_URL": "ollama"}),
			wantErr: "OLLAMA_ENDPOINT_URL must be an absolute http(s) URL",
		},
		{
			name:    "ollama endpoint is a relative path",
			env:     ollamaEnv(map[string]string{"OLLAMA_ENDPOINT_URL": "/api/embed"}),
			wantErr: "OLLAMA_ENDPOINT_URL must be an absolute http(s) URL",
		},
		{
			name:    "ollama endpoint has a non-http scheme",
			env:     ollamaEnv(map[string]string{"OLLAMA_ENDPOINT_URL": "ftp://ollama/api/embed"}),
			wantErr: "OLLAMA_ENDPOINT_URL must be an absolute http(s) URL",
		},
		{
			name:    "ollama endpoint fails to parse",
			env:     ollamaEnv(map[string]string{"OLLAMA_ENDPOINT_URL": "http://bad host/"}),
			wantErr: "OLLAMA_ENDPOINT_URL must be an absolute http(s) URL",
		},
		{
			name: "https ollama endpoint",
			env:  ollamaEnv(map[string]string{"OLLAMA_ENDPOINT_URL": "https://ollama.example.com/api/embed"}),
			want: func() *Config {
				c := defaultOllamaConfig(defaultLog)
				c.Ollama.Endpoint = "https://ollama.example.com/api/embed"
				return c
			}(),
		},
		{
			name:    "ollama model is missing",
			env:     ollamaEnv(map[string]string{"OLLAMA_MODEL": ""}),
			wantErr: "missing OLLAMA_MODEL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setEnv(t, tt.env)

			got, err := Configure()

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tt.wantErr)
				assert.Nil(t, got)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
