package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type Metrics struct {
	Requests *prometheus.CounterVec
	Duration *prometheus.HistogramVec
}

func New(reg prometheus.Registerer) *Metrics {
	c := promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
		Name: "embeddings_requests_total",
		Help: "The total number of processed requests",
	}, []string{"transport", "method", "status"})
	h := promauto.With(reg).NewHistogramVec(prometheus.HistogramOpts{
		Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10, 30},
		Name:    "embeddings_request_duration_seconds",
		Help:    "The duration for a request",
	}, []string{"transport", "method"})
	return &Metrics{
		Requests: c,
		Duration: h,
	}
}
