package web

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/R-Jena/VXR-Thesis/internal/engine"
	"github.com/R-Jena/VXR-Thesis/internal/telemetry"
)

// Handler holds references to the execution engine and telemetry monitor.
type Handler struct {
	executor *engine.Executor
	monitor  *telemetry.Monitor
	writer   *telemetry.CSVWriter
}

// NewHandler creates a new web request handler.
func NewHandler(executor *engine.Executor, monitor *telemetry.Monitor, writer *telemetry.CSVWriter) *Handler {
	return &Handler{
		executor: executor,
		monitor:  monitor,
		writer:   writer,
	}
}

// HandleIndex serves the main dashboard HTML page.
func (h *Handler) HandleIndex(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "web/templates/index.html")
}

// HandleMetricsJSON returns a raw JSON snapshot of telemetry.
func (h *Handler) HandleMetricsJSON(w http.ResponseWriter, r *http.Request) {
	snap := h.monitor.Snapshot()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(snap)
}

// HandleLatencyPartial renders the HTMX fragment for latency metrics.
func (h *Handler) HandleLatencyPartial(w http.ResponseWriter, r *http.Request) {
	snap := h.monitor.Snapshot()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `
	<div class="metrics-grid">
		<div class="metric-card">
			<span class="label">p50 Latency</span>
			<span class="value">%.2f µs</span>
		</div>
		<div class="metric-card">
			<span class="label">p95 Latency</span>
			<span class="value">%.2f µs</span>
		</div>
		<div class="metric-card">
			<span class="label">p99 Tail Latency</span>
			<span class="value accent">%.2f µs</span>
		</div>
		<div class="metric-card">
			<span class="label">Throughput</span>
			<span class="value">%.2f MB/s</span>
		</div>
	</div>
	`, snap.P50LatencyUs, snap.P95LatencyUs, snap.P99LatencyUs, snap.ThroughputMBps)
}

// HandleMemoryPartial renders the HTMX fragment for memory consumption.
func (h *Handler) HandleMemoryPartial(w http.ResponseWriter, r *http.Request) {
	snap := h.monitor.Snapshot()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `
	<div class="memory-status">
		<div class="gauge-container">
			<div class="gauge-bar" style="width: %.1f%%"></div>
		</div>
		<div class="memory-details">
			<span>Heap Alloc: <strong>%.2f MB</strong></span>
			<span>Sys Memory: <strong>%.2f MB</strong></span>
			<span>Edge Ceiling: <strong>512.00 MB</strong> (%.1f%%)</span>
			<span>GC Cycles: <strong>%d</strong></span>
		</div>
	</div>
	`, snap.MemoryPressurePct, snap.HeapAllocMB, snap.SysMemMB, snap.MemoryPressurePct, snap.NumGC)
}
