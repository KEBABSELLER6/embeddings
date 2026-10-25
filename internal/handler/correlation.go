package handler

import (
	"net/http"

	"uuid"

	"github.com/KEBABSELLER6/embeddings/internal/logging"
)

const maxCorrelationIDLen = 128

func validCorrelationID(id string) bool {
	if id == "" || len(id) > maxCorrelationIDLen {
		return false
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		switch {
		case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', '0' <= c && c <= '9':
		case c == '-', c == '_', c == '.':
		default:
			return false
		}
	}
	return true
}

func CorrelationID(next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Correlation-ID")
		if !validCorrelationID(id) {
			id = uuid.NewV7().String()
		}

		ctx := logging.WithCorrelationID(r.Context(), id)
		w.Header().Set("X-Correlation-ID", id)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}
