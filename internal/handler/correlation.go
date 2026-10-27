package handler

import (
	"net/http"

	"uuid"

	"github.com/KEBABSELLER6/embeddings/internal/logging"
)

func CorrelationID(next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Correlation-ID")
		if !logging.ValidCorrelationID(id) {
			id = uuid.NewV7().String()
		}

		ctx := logging.WithCorrelationID(r.Context(), id)
		w.Header().Set("X-Correlation-ID", id)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}
