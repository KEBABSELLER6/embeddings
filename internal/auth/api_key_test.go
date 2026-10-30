package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAPIKeyServiceAuthenticate(t *testing.T) {
	const key = "s3cret-key"

	tests := []struct {
		name     string
		incoming string
		want     bool
	}{
		{"exact key", key, true},
		{"wrong key", "not-the-key", false},
		{"empty key", "", false},
		{"prefix of the key", key[:len(key)-1], false},
		{"key with extra suffix", key + "x", false},
		{"different case", "S3CRET-KEY", false},
		{"surrounding whitespace", " " + key + " ", false},
	}

	svc := NewAPIKeyService(key)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, svc.Authenticate(tt.incoming))
		})
	}
}
