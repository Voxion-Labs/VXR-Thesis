package web

import (
	"net/http"

	"github.com/R-Jena/VXR-Thesis/internal/engine"
	"github.com/R-Jena/VXR-Thesis/internal/telemetry"
)

// NewRouter registers HTTP routes for the dashboard and static assets.
func NewRouter(executor *engine.Executor, monitor *telemetry.Monitor, writer *telemetry.CSVWriter) http.Handler {
	mux := http.NewServeMux()
	handler := NewHandler(executor, monitor, writer)

	// Dashboard page
	mux.HandleFunc("/", handler.HandleIndex)

	// Health check endpoints for cloud hosting (Render, Kubernetes, Docker)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	// HTMX polling fragments & API endpoints
	mux.HandleFunc("/api/metrics/latency", handler.HandleLatencyPartial)
	mux.HandleFunc("/api/metrics/memory", handler.HandleMemoryPartial)
	mux.HandleFunc("/api/metrics/json", handler.HandleMetricsJSON)
	mux.HandleFunc("/api/benchmark/trigger", handler.HandleTriggerBenchmark)

	// Static assets (CSS, static wasm)
	fs := http.FileServer(http.Dir("web/static"))
	mux.Handle("/static/", http.StripPrefix("/static/", fs))

	return mux
}
