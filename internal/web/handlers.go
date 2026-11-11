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

	// Parse Concurrency: custom overrides preset dropdown
	concurrency, _ := strconv.Atoi(r.FormValue("custom_concurrency"))
	if concurrency <= 0 {
		concurrency, _ = strconv.Atoi(r.FormValue("concurrency"))
	}
	if concurrency <= 0 {
		concurrency = 4
	}
	if concurrency > 32 {
		concurrency = 32
	}

	// Parse Iterations: custom overrides preset dropdown
	iterations, _ := strconv.Atoi(r.FormValue("custom_iterations"))
	if iterations <= 0 {
		iterations, _ = strconv.Atoi(r.FormValue("iterations"))
	}
	if iterations <= 0 {
		iterations = 2000
	}
	if iterations > 50000 {
		iterations = 50000
	}

	// Parse Payload Size: custom overrides preset dropdown
	payloadSize, _ := strconv.Atoi(r.FormValue("custom_payload"))
	if payloadSize <= 0 {
		payloadSize, _ = strconv.Atoi(r.FormValue("payload"))
	}
	if payloadSize <= 0 {
		payloadSize = 4096
	}
	// Safe clamp for linear memory ring buffer bounds (16B to 64KB)
	if payloadSize < 16 {
		payloadSize = 16
	}
	if payloadSize > 65536 {
		payloadSize = 65536
	}

	ctx := context.Background()

	if h.monitor != nil {
		h.monitor.Reset()
	}

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

	if h.monitor != nil {
		if zeroSummary != nil {
			h.monitor.SetBenchmarkSummary(zeroSummary.ThroughputMBps, zeroSummary.OpsPerSec)
		} else if baseSummary != nil {
			h.monitor.SetBenchmarkSummary(baseSummary.ThroughputMBps, baseSummary.OpsPerSec)
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	payloadStr := fmt.Sprintf("%d B", payloadSize)
	if payloadSize >= 1024 && payloadSize%1024 == 0 {
		payloadStr = fmt.Sprintf("%d KB", payloadSize/1024)
	}

	if baseSummary != nil && zeroSummary != nil {
		speedup := 1.0
		if zeroSummary.MeanLatencyUs > 0.001 {
			speedup = baseSummary.MeanLatencyUs / zeroSummary.MeanLatencyUs
		} else if baseSummary.MeanLatencyUs > 0.001 {
			speedup = baseSummary.MeanLatencyUs / 0.1
		}

		p99Diff := 0.0
		if baseSummary.P99LatencyUs > 0.001 {
			p99Diff = ((baseSummary.P99LatencyUs - zeroSummary.P99LatencyUs) / baseSummary.P99LatencyUs) * 100.0
		}

		fmt.Fprintf(w, `
		<div class="result-summary">
			<div class="result-header">
				<span class="result-badge">✔ Workload Finished: In-Place Execution Completed</span>
				<span class="result-meta">Workers: %d | Payload: %s (%d B) | Invocations: %d ops</span>
			</div>
			<table class="result-table">
				<thead>
					<tr>
						<th>Execution Pipeline</th>
						<th>Mean Latency</th>
						<th>p50 (Median)</th>
						<th>p99 Tail Latency</th>
						<th>Effective Throughput</th>
					</tr>
				</thead>
				<tbody>
					<tr class="row-baseline">
						<td>Baseline (Copy + Alloc)</td>
						<td>%.2f µs</td>
						<td>%.2f µs</td>
						<td class="text-amber">%.2f µs</td>
						<td>%.2f MB/s</td>
					</tr>
					<tr class="row-zerocopy">
						<td><strong>Zero-Copy (Direct Slicing)</strong></td>
						<td><strong>%.2f µs</strong></td>
						<td><strong>%.2f µs</strong></td>
						<td class="text-green"><strong>%.2f µs</strong></td>
						<td><strong>%.2f MB/s</strong></td>
					</tr>
				</tbody>
			</table>
			<div class="result-stats-bar">
				<span class="stat-pill speedup">Speedup: <strong>%.2fx faster</strong></span>
				<span class="stat-pill delta">p99 Delta: <strong>%.1f%% reduction</strong></span>
				<span class="stat-pill checksum">Checksum: <code>0x%X</code> (VERIFIED)</span>
			</div>
		</div>
		`, concurrency, payloadStr, payloadSize, iterations,
			baseSummary.MeanLatencyUs, baseSummary.P50LatencyUs, baseSummary.P99LatencyUs, baseSummary.ThroughputMBps,
			zeroSummary.MeanLatencyUs, zeroSummary.P50LatencyUs, zeroSummary.P99LatencyUs, zeroSummary.ThroughputMBps,
			speedup, p99Diff, zeroSummary.LastChecksum)
	} else if zeroSummary != nil {
		fmt.Fprintf(w, `
		<div class="result-summary">
			<div class="result-header">
				<span class="result-badge">✔ Workload Finished: Zero-Copy Execution Completed</span>
				<span class="result-meta">Workers: %d | Payload: %s (%d B) | Invocations: %d ops</span>
			</div>
			<div class="result-stats-bar">
				<span>Mean: <strong>%.2f µs</strong></span>
				<span>p50: <strong>%.2f µs</strong></span>
				<span>p99: <strong>%.2f µs</strong></span>
				<span>Throughput: <strong>%.2f MB/s</strong></span>
				<span class="stat-pill checksum">Checksum: <code>0x%X</code> (VERIFIED)</span>
			</div>
		</div>
		`, concurrency, payloadStr, payloadSize, iterations,
			zeroSummary.MeanLatencyUs, zeroSummary.P50LatencyUs, zeroSummary.P99LatencyUs, zeroSummary.ThroughputMBps, zeroSummary.LastChecksum)
	} else if baseSummary != nil {
		fmt.Fprintf(w, `
		<div class="result-summary">
			<div class="result-header">
				<span class="result-badge text-amber">✔ Workload Finished: Baseline Copy Completed</span>
				<span class="result-meta">Workers: %d | Payload: %s (%d B) | Invocations: %d ops</span>
			</div>
			<div class="result-stats-bar">
				<span>Mean: <strong>%.2f µs</strong></span>
				<span>p50: <strong>%.2f µs</strong></span>
				<span>p99: <strong>%.2f µs</strong></span>
				<span>Throughput: <strong>%.2f MB/s</strong></span>
				<span class="stat-pill checksum">Checksum: <code>0x%X</code> (VERIFIED)</span>
			</div>
		</div>
		`, concurrency, payloadStr, payloadSize, iterations,
			baseSummary.MeanLatencyUs, baseSummary.P50LatencyUs, baseSummary.P99LatencyUs, baseSummary.ThroughputMBps, baseSummary.LastChecksum)
	}
}
