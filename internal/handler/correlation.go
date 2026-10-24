package handler

import (
	"context"
	"net/http"

	"uuid"

	"github.com/KEBABSELLER6/embeddings/internal/common"
)

func CorrelationID(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Correlation-ID")
		if id == "" {
			id = uuid.NewV7().String()
		}

		ctx := context.WithValue(r.Context(), common.CorrelationKey, id)
		w.Header().Set("X-Correlation-ID", id)
		next(w, r.WithContext(ctx))
	}
}
