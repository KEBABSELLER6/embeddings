package handler

import (
	"net/http"
	"sync/atomic"
)

type Readiness struct{ draining atomic.Bool }

func NewReadiness() *Readiness {
	return &Readiness{}
}

func (r *Readiness) SetDraining() {
	r.draining.Store(true)
}

func (r *Readiness) HandleReadyRequest(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.draining.Load() {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"status":"down"}`))
		return
	}

	w.Write([]byte(`{"status":"ok"}`))
}
