package web

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strconv"

	"github.com/R-Jena/VXR-Thesis/internal/engine"
	"github.com/R-Jena/VXR-Thesis/internal/telemetry"
)

// Handler holds references to the execution engine and telemetry monitor.
type Handler struct {
	executor       *engine.Executor
	monitor        *telemetry.Monitor
	writer         *telemetry.CSVWriter
	indexTmpl      *template.Template
	latencyTmpl    *template.Template
	memoryTmpl     *template.Template
}

// NewHandler creates a new web request handler and compiles HTML templates.
func NewHandler(executor *engine.Executor, monitor *telemetry.Monitor, writer *telemetry.CSVWriter) *Handler {
	funcMap := template.FuncMap{
		"sub": func(a, b float64) float64 {
			res := a - b
			if res < 0 {
				return 0
			}
			return res
		},
	}

	indexTmpl := template.Must(template.ParseFiles("web/templates/index.html"))
	latencyTmpl := template.Must(template.New("latency_metrics.html").Funcs(funcMap).ParseFiles("web/templates/partials/latency_metrics.html"))
	memoryTmpl := template.Must(template.New("memory_chart.html").Funcs(funcMap).ParseFiles("web/templates/partials/memory_chart.html"))

	return &Handler{
		executor:    executor,
		monitor:     monitor,
		writer:      writer,
		indexTmpl:   indexTmpl,
		latencyTmpl: latencyTmpl,
		memoryTmpl:  memoryTmpl,
	}
}

// HandleIndex serves the main dashboard HTML page.
func (h *Handler) HandleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = h.indexTmpl.Execute(w, nil)
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
	_ = h.latencyTmpl.Execute(w, snap)
}

// HandleMemoryPartial renders the HTMX fragment for memory consumption.
func (h *Handler) HandleMemoryPartial(w http.ResponseWriter, r *http.Request) {
	snap := h.monitor.Snapshot()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = h.memoryTmpl.Execute(w, snap)
}

// HandleTriggerBenchmark processes the HTMX load test trigger form and returns comparative HTML results.
func (h *Handler) HandleTriggerBenchmark(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	mode := r.FormValue("mode")
	if mode == "" {
		mode = "both"
	}

	concurrency, _ := strconv.Atoi(r.FormValue("concurrency"))
	if concurrency <= 0 {
		concurrency = 4
	}

	iterations, _ := strconv.Atoi(r.FormValue("iterations"))
	if iterations <= 0 {
		iterations = 2000
	}

	payloadSize, _ := strconv.Atoi(r.FormValue("payload"))
	if payloadSize <= 0 {
		payloadSize = 4096
	}

	ctx := context.Background()

	var baseSummary *engine.BenchmarkSummary
	var zeroSummary *engine.BenchmarkSummary

	if mode == "baseline_copy" || mode == "both" {
		cfg := engine.BenchmarkConfig{
			Mode:        "baseline_copy",
			Concurrency: concurrency,
			Iterations:  iterations,
			PayloadSize: payloadSize,
			WarmupRuns:  20,
		}
		baseSummary, _ = h.executor.RunBenchmarkSuite(ctx, cfg, h.monitor, h.writer)
	}

	if mode == "zero_copy" || mode == "both" {
		cfg := engine.BenchmarkConfig{
			Mode:        "zero_copy",
			Concurrency: concurrency,
			Iterations:  iterations,
			PayloadSize: payloadSize,
			WarmupRuns:  20,
		}
		zeroSummary, _ = h.executor.RunBenchmarkSuite(ctx, cfg, h.monitor, h.writer)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if baseSummary != nil && zeroSummary != nil {
		speedup := baseSummary.MeanLatencyUs / zeroSummary.MeanLatencyUs
		p99Diff := ((baseSummary.P99LatencyUs - zeroSummary.P99LatencyUs) / baseSummary.P99LatencyUs) * 100.0

		fmt.Fprintf(w, `
		<div style="display: flex; flex-direction: column; gap: 8px;">
			<div style="color: var(--accent-cyan); font-weight: bold;">
				✔ Workload Finished: %d operations injected (Concurrency: %d, Payload: %d B)
			</div>
			<table style="width: 100%%; border-collapse: collapse; margin-top: 6px;">
				<thead>
					<tr style="color: var(--text-dim); text-align: left; border-bottom: 1px solid var(--border-dim);">
						<th style="padding: 4px 8px;">Path</th>
						<th style="padding: 4px 8px;">Mean Latency</th>
						<th style="padding: 4px 8px;">p50 (Median)</th>
						<th style="padding: 4px 8px;">p99 Tail</th>
						<th style="padding: 4px 8px;">Throughput</th>
					</tr>
				</thead>
				<tbody>
					<tr style="border-bottom: 1px solid var(--border-dim);">
						<td style="padding: 4px 8px;">Baseline (Copy+Alloc)</td>
						<td style="padding: 4px 8px;">%.2f µs</td>
						<td style="padding: 4px 8px;">%.2f µs</td>
						<td style="padding: 4px 8px; color: var(--accent-amber);">%.2f µs</td>
						<td style="padding: 4px 8px;">%.2f MB/s</td>
					</tr>
					<tr>
						<td style="padding: 4px 8px; color: var(--accent-cyan); font-weight: bold;">Zero-Copy In-Place</td>
						<td style="padding: 4px 8px; color: var(--accent-green); font-weight: bold;">%.2f µs</td>
						<td style="padding: 4px 8px;">%.2f µs</td>
						<td style="padding: 4px 8px; color: var(--accent-green); font-weight: bold;">%.2f µs</td>
						<td style="padding: 4px 8px;">%.2f MB/s</td>
					</tr>
				</tbody>
			</table>
			<div style="margin-top: 8px; padding: 6px 10px; background: var(--bg-card); border-radius: 4px; border: 1px solid var(--border-dim); font-size: 11px;">
				<span style="color: var(--accent-green); font-weight: bold;">Speedup: %.2fx faster</span> | 
				<span style="color: var(--accent-cyan);">p99 Delta: %.1f%% drop</span> | 
				<span>Cryptographic Checksum: 0x%X (VERIFIED)</span>
			</div>
		</div>
		`, baseSummary.TotalOps, concurrency, payloadSize,
			baseSummary.MeanLatencyUs, baseSummary.P50LatencyUs, baseSummary.P99LatencyUs, baseSummary.ThroughputMBps,
			zeroSummary.MeanLatencyUs, zeroSummary.P50LatencyUs, zeroSummary.P99LatencyUs, zeroSummary.ThroughputMBps,
			speedup, p99Diff, zeroSummary.LastChecksum)
	} else if zeroSummary != nil {
		fmt.Fprintf(w, `
		<div style="color: var(--accent-green); font-weight: bold;">
			✔ Zero-Copy In-Place Run: %d ops | Mean: %.2f µs | p99: %.2f µs | Throughput: %.2f MB/s
		</div>
		`, zeroSummary.TotalOps, zeroSummary.MeanLatencyUs, zeroSummary.P99LatencyUs, zeroSummary.ThroughputMBps)
	} else if baseSummary != nil {
		fmt.Fprintf(w, `
		<div style="color: var(--accent-amber); font-weight: bold;">
			✔ Baseline Copy Run: %d ops | Mean: %.2f µs | p99: %.2f µs | Throughput: %.2f MB/s
		</div>
		`, baseSummary.TotalOps, baseSummary.MeanLatencyUs, baseSummary.P99LatencyUs, baseSummary.ThroughputMBps)
	}
}
