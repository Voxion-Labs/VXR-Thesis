package engine

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/R-Jena/VXR-Thesis/internal/telemetry"
)

// ExecutionResult captures the empirical output of an individual invocation.
type ExecutionResult struct {
	Mode           string
	DurationUs     float64
	BytesProcessed int
	Checksum       uint64
	HeapAllocBytes uint64
}

// BenchmarkConfig defines the parameters for an empirical test run.
type BenchmarkConfig struct {
	Mode         string // "baseline_copy", "zero_copy", or "both"
	Concurrency  int    // Number of concurrent worker goroutines
	Iterations   int    // Total operations per worker
	PayloadSize  int    // Payload size in bytes (e.g. 64, 4096, 65536)
	WarmupRuns   int    // Warmup iterations discarded from measurements
}

// BenchmarkSummary holds aggregate metrics for a completed benchmark run.
type BenchmarkSummary struct {
	Mode           string    `json:"mode"`
	TotalOps       int       `json:"total_ops"`
	Concurrency    int       `json:"concurrency"`
	PayloadSize    int       `json:"payload_size"`
	TotalDuration  float64   `json:"total_duration_sec"`
	ThroughputMBps float64   `json:"throughput_mbps"`
	OpsPerSec      float64   `json:"ops_per_sec"`
	P50LatencyUs   float64   `json:"p50_latency_us"`
	P90LatencyUs   float64   `json:"p90_latency_us"`
	P95LatencyUs   float64   `json:"p95_latency_us"`
	P99LatencyUs   float64   `json:"p99_latency_us"`
	MeanLatencyUs  float64   `json:"mean_latency_us"`
	MinLatencyUs   float64   `json:"min_latency_us"`
	MaxLatencyUs   float64   `json:"max_latency_us"`
	HeapAllocMB    float64   `json:"heap_alloc_mb"`
	SysMemMB       float64   `json:"sys_mem_mb"`
	LastChecksum   uint64    `json:"last_checksum"`
}

// Executor orchestrates execution runs and concurrent workloads against the Wasm instance.
type Executor struct {
	instance *WasmInstance
	mu       sync.Mutex // Serializes access to guest linear memory regions
}

// NewExecutor creates a new benchmark executor for a Wasm instance.
func NewExecutor(instance *WasmInstance) *Executor {
	return &Executor{
		instance: instance,
	}
}

// ExecuteCopy runs the baseline path:
// 1. Dynamic guest memory allocation via vxr_alloc_arena
// 2. Host-to-guest payload memcpy
// 3. Output buffer guest allocation
// 4. vxr_process_copy execution
// 5. Guest-to-host result memcpy
func (e *Executor) ExecuteCopy(ctx context.Context, payload []byte) (ExecutionResult, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	n := len(payload)
	if n == 0 {
		return ExecutionResult{}, fmt.Errorf("payload cannot be empty")
	}

	start := time.Now()

	// 1. Allocate guest memory for input
	resIn, err := e.instance.fnAllocArena.Call(ctx, uint64(n))
	if err != nil || len(resIn) == 0 || resIn[0] == 0 {
		// Arena full: reset and retry
		_, _ = e.instance.fnResetArena.Call(ctx)
		resIn, err = e.instance.fnAllocArena.Call(ctx, uint64(n))
		if err != nil || len(resIn) == 0 || resIn[0] == 0 {
			return ExecutionResult{}, fmt.Errorf("failed to allocate guest memory for copy input")
		}
	}
	inPtr := uint32(resIn[0])

	// 2. Host -> Guest boundary memcpy
	if !e.instance.memory.Write(inPtr, payload) {
		return ExecutionResult{}, fmt.Errorf("failed to write payload to guest memory at %d", inPtr)
	}

	// 3. Allocate guest memory for output
	resOut, err := e.instance.fnAllocArena.Call(ctx, uint64(n))
	if err != nil || len(resOut) == 0 || resOut[0] == 0 {
		return ExecutionResult{}, fmt.Errorf("failed to allocate guest memory for copy output")
	}
	outPtr := uint32(resOut[0])

	// 4. Execute compute kernel
	resExec, err := e.instance.fnProcessCopy.Call(ctx, uint64(inPtr), uint64(n), uint64(outPtr))
	if err != nil || len(resExec) == 0 {
		return ExecutionResult{}, fmt.Errorf("vxr_process_copy call failed: %w", err)
	}
	checksum := resExec[0]

	// 5. Guest -> Host boundary memcpy
	outSlice, ok := e.instance.memory.Read(outPtr, uint32(n))
	if !ok || len(outSlice) != n {
		return ExecutionResult{}, fmt.Errorf("failed to read back processed payload from guest memory")
	}

	durationUs := float64(time.Since(start).Nanoseconds()) / 1000.0

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	return ExecutionResult{
		Mode:           "baseline_copy",
		DurationUs:     durationUs,
		BytesProcessed: n,
		Checksum:       checksum,
		HeapAllocBytes: mem.Alloc,
	}, nil
}

// ExecuteZeroCopy runs the zero-copy path:
// 1. Host writes directly into pre-mapped guest linear memory ring buffer at specified offset
// 2. vxr_process_zero_copy transforms in-place (0 guest allocations, 0 boundary copies)
// 3. Checksum returned directly via register
func (e *Executor) ExecuteZeroCopy(ctx context.Context, offset uint32, payload []byte) (ExecutionResult, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	n := len(payload)
	if n == 0 {
		return ExecutionResult{}, fmt.Errorf("payload cannot be empty")
	}

	if offset+uint32(n) > e.instance.ringCapacity {
		offset = 0 // Wrap to ring head
	}

	start := time.Now()

	// 1. Direct memory write into guest linear memory
	targetAddr := e.instance.ringBufferPtr + offset
	if !e.instance.memory.Write(targetAddr, payload) {
		return ExecutionResult{}, fmt.Errorf("failed direct linear memory write at %d", targetAddr)
	}

	// 2. In-place execution without allocation or copies
	resExec, err := e.instance.fnProcessZeroCopy.Call(ctx, uint64(offset), uint64(n))
	if err != nil || len(resExec) == 0 {
		return ExecutionResult{}, fmt.Errorf("vxr_process_zero_copy call failed: %w", err)
	}
	checksum := resExec[0]

	durationUs := float64(time.Since(start).Nanoseconds()) / 1000.0

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	return ExecutionResult{
		Mode:           "zero_copy",
		DurationUs:     durationUs,
		BytesProcessed: n,
		Checksum:       checksum,
		HeapAllocBytes: mem.Alloc,
	}, nil
}

// RunBenchmarkSuite executes a structured load test and streams data to monitor & CSV writer.
func (e *Executor) RunBenchmarkSuite(
	ctx context.Context,
	cfg BenchmarkConfig,
	mon *telemetry.Monitor,
	writer *telemetry.CSVWriter,
) (*BenchmarkSummary, error) {
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 1
	}
	if cfg.Iterations <= 0 {
		cfg.Iterations = 1000
	}
	if cfg.PayloadSize <= 0 {
		cfg.PayloadSize = 4096
	}

	// Generate deterministic benchmark payload
	payload := make([]byte, cfg.PayloadSize)
	for i := range payload {
		payload[i] = byte((i*37 + 11) & 0xFF)
	}

	// Warmup runs
	for i := 0; i < cfg.WarmupRuns; i++ {
		if cfg.Mode == "baseline_copy" {
			_, _ = e.ExecuteCopy(ctx, payload)
		} else {
			_, _ = e.ExecuteZeroCopy(ctx, 0, payload)
		}
	}

	if mon != nil {
		mon.Reset()
	}

	var completedOps uint64
	var wg sync.WaitGroup
	startSuite := time.Now()

	opsPerWorker := cfg.Iterations / cfg.Concurrency
	if opsPerWorker < 1 {
		opsPerWorker = 1
	}

	for w := 0; w < cfg.Concurrency; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			// Offset for each worker within ring buffer to demonstrate spatial isolation
			workerOffset := uint32((workerID * cfg.PayloadSize) % int(e.instance.ringCapacity-uint32(cfg.PayloadSize)))

			for i := 0; i < opsPerWorker; i++ {
				select {
				case <-ctx.Done():
					return
				default:
				}

				var res ExecutionResult
				var err error

				if cfg.Mode == "baseline_copy" {
					res, err = e.ExecuteCopy(ctx, payload)
				} else {
					res, err = e.ExecuteZeroCopy(ctx, workerOffset, payload)
				}

				if err != nil {
					continue
				}

				atomic.AddUint64(&completedOps, 1)

				if mon != nil {
					mon.RecordObservation(res.Mode, res.DurationUs, res.BytesProcessed, res.Checksum)
				}

				if writer != nil {
					var mem runtime.MemStats
					runtime.ReadMemStats(&mem)
					writer.Log(telemetry.LogRecord{
						Timestamp:      time.Now(),
						Mode:           res.Mode,
						PayloadSize:    res.BytesProcessed,
						Concurrency:    cfg.Concurrency,
						LatencyUs:      res.DurationUs,
						Checksum:       res.Checksum,
						HeapAllocBytes: mem.Alloc,
						SysMemBytes:    mem.Sys,
						NumGC:          mem.NumGC,
					})
				}
			}
		}(w)
	}

	wg.Wait()
	elapsedTotal := time.Since(startSuite).Seconds()

	var snap telemetry.MetricsSnapshot
	if mon != nil {
		snap = mon.Snapshot()
	}

	summary := &BenchmarkSummary{
		Mode:           cfg.Mode,
		TotalOps:       int(completedOps),
		Concurrency:    cfg.Concurrency,
		PayloadSize:    cfg.PayloadSize,
		TotalDuration:  elapsedTotal,
		ThroughputMBps: (float64(completedOps*uint64(cfg.PayloadSize)) / (1024 * 1024)) / elapsedTotal,
		OpsPerSec:      float64(completedOps) / elapsedTotal,
		P50LatencyUs:   snap.P50LatencyUs,
		P90LatencyUs:   snap.P90LatencyUs,
		P95LatencyUs:   snap.P95LatencyUs,
		P99LatencyUs:   snap.P99LatencyUs,
		MeanLatencyUs:  snap.MeanLatencyUs,
		MinLatencyUs:   snap.MinLatencyUs,
		MaxLatencyUs:   snap.MaxLatencyUs,
		HeapAllocMB:    snap.HeapAllocMB,
		SysMemMB:       snap.SysMemMB,
		LastChecksum:   snap.LastChecksum,
	}

	return summary, nil
}
