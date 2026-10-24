package config

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var envKeys = []string{
	"PORT",
	"MAX_BODY_SIZE",
	"PROVIDER",
	"OLLAMA_MODEL",
	"OLLAMA_ENDPOINT_URL",
	"LOG_LEVEL",
	"LOG_FORMAT",
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

	// defaultOllamaConfig is what ollamaEnv(nil) produces, with log settings swapped in.
	defaultOllamaConfig := func(log LogConfig) *Config {
		return &Config{
			Port:        "8080",
			MaxBodySize: 512000,
			Provider:    ProviderOllama,
			Ollama: &OllamaConfig{
				Model:    "nomic-embed-text",
				Endpoint: "http://localhost:11434/api/embed",
			},
			Log: log,
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
				MaxBodySize: 512000,
				Provider:    ProviderOllama,
				Ollama: &OllamaConfig{
					Model:    "nomic-embed-text",
					Endpoint: "http://localhost:11434/api/embed",
				},
				Log: defaultLog,
			},
		},
		{
			name: "custom port and body size",
			env:  ollamaEnv(map[string]string{"PORT": "9000", "MAX_BODY_SIZE": "256000"}),
			want: &Config{
				Port:        "9000",
				MaxBodySize: 256000,
				Provider:    ProviderOllama,
				Ollama: &OllamaConfig{
					Model:    "nomic-embed-text",
					Endpoint: "http://localhost:11434/api/embed",
				},
				Log: defaultLog,
			},
		},
		{
			name: "custom ollama model and endpoint",
			env: ollamaEnv(map[string]string{
				"OLLAMA_MODEL":        "mxbai-embed-large",
				"OLLAMA_ENDPOINT_URL": "http://ollama:11434/api/embed",
			}),
			want: &Config{
				Port:        "8080",
				MaxBodySize: 512000,
				Provider:    ProviderOllama,
				Ollama: &OllamaConfig{
					Model:    "mxbai-embed-large",
					Endpoint: "http://ollama:11434/api/embed",
				},
				Log: defaultLog,
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
