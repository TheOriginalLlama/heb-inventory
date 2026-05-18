package handlers

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	requestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total HTTP requests.",
		},
		[]string{"method", "status"},
	)
	requestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request duration.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method"},
	)
)

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func Observability(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

			defer func() {
				if p := recover(); p != nil {
					logger.Error("panic recovered", "panic", p, "path", r.URL.Path)
					rec.status = http.StatusInternalServerError
					http.Error(w, "internal error", http.StatusInternalServerError)
				}
				dur := time.Since(start)
				requestsTotal.WithLabelValues(r.Method, http.StatusText(rec.status)).Inc()
				requestDuration.WithLabelValues(r.Method).Observe(dur.Seconds())
				logger.Info("http",
					"method", r.Method,
					"path", r.URL.Path,
					"status", rec.status,
					"duration_ms", dur.Milliseconds(),
				)
			}()

			next.ServeHTTP(rec, r)
		})
	}
}
