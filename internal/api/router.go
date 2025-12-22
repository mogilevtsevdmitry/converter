package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

// NewRouter creates a new API router
func NewRouter(h *Handler, logger *zap.Logger) *chi.Mux {
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))
	r.Use(requestLogger(logger))

	// Health endpoints
	r.Get("/healthz", h.HealthCheck)
	r.Get("/readyz", h.ReadyCheck)

	// Metrics endpoint
	r.Handle("/metrics", promhttp.Handler())

	// API routes
	r.Route("/v1", func(r chi.Router) {
		r.Route("/jobs", func(r chi.Router) {
			r.Post("/", h.CreateJob)
			r.Get("/{jobId}", h.GetJob)
			r.Post("/{jobId}/cancel", h.CancelJob)
			r.Get("/{jobId}/artifacts", h.GetArtifacts)
		})
	})

	return r
}

// requestLogger logs HTTP requests
func requestLogger(logger *zap.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			defer func() {
				logger.Info("request",
					zap.String("method", r.Method),
					zap.String("path", r.URL.Path),
					zap.Int("status", ww.Status()),
					zap.Duration("duration", time.Since(start)),
					zap.String("requestId", middleware.GetReqID(r.Context())),
				)
			}()

			next.ServeHTTP(ww, r)
		})
	}
}
