package telemetry

import (
	"math"
	"runtime"
	"sort"
	"sync"
	"time"
)

// MetricsSnapshot represents a point-in-time state of the telemetry monitor.
type MetricsSnapshot struct {
	Timestamp          time.Time `json:"timestamp"`
	TotalOps           uint64    `json:"total_ops"`
	BaselineOps        uint64    `json:"baseline_ops"`
	ZeroCopyOps        uint64    `json:"zero_copy_ops"`
	BytesProcessed     uint64    `json:"bytes_processed"`
	ThroughputMBps     float64   `json:"throughput_mbps"`
	OpsPerSec          float64   `json:"ops_per_sec"`
	P50LatencyUs       float64   `json:"p50_latency_us"`
	P90LatencyUs       float64   `json:"p90_latency_us"`
	P95LatencyUs       float64   `json:"p95_latency_us"`
	P99LatencyUs       float64   `json:"p99_latency_us"`
	MeanLatencyUs      float64   `json:"mean_latency_us"`
	MinLatencyUs       float64   `json:"min_latency_us"`
	MaxLatencyUs       float64   `json:"max_latency_us"`
	HeapAllocBytes     uint64    `json:"heap_alloc_bytes"`
	HeapAllocMB        float64   `json:"heap_alloc_mb"`
	SysMemBytes        uint64    `json:"sys_mem_bytes"`
	SysMemMB           float64   `json:"sys_mem_mb"`
	NumGC              uint32    `json:"num_gc"`
	LastChecksum       uint64    `json:"last_checksum"`
	EdgeMemoryLimitMB  float64   `json:"edge_memory_limit_mb"` // 512MB ceiling
	MemoryPressurePct  float64   `json:"memory_pressure_pct"`
}

// Monitor tracks high-resolution latency histograms and Go runtime memory allocations.
type Monitor struct {
	mu             sync.RWMutex
	startTime      time.Time
	totalOps       uint64
	baselineOps    uint64
	zeroCopyOps    uint64
	bytesProcessed uint64
	lastChecksum   uint64

	// Fixed rolling reservoir of latencies in microseconds (up to 20,000 samples)
	maxSamples          int
	latenciesUs         []float64
	sampleIndex         int
	baselineLatenciesUs []float64
	zeroCopyLatenciesUs []float64

	// Active burst metrics from latest benchmark execution
	latestThroughputMBps float64
	latestOpsPerSec      float64
}

// NewMonitor initializes a thread-safe systems monitor.
func NewMonitor(reservoirSize int) *Monitor {
	if reservoirSize <= 0 {
		reservoirSize = 20000
	}
	return &Monitor{
		startTime:           time.Now(),
		maxSamples:          reservoirSize,
		latenciesUs:         make([]float64, 0, reservoirSize),
		baselineLatenciesUs: make([]float64, 0, reservoirSize/2),
		zeroCopyLatenciesUs: make([]float64, 0, reservoirSize/2),
	}
}

// SetBenchmarkSummary updates the monitor with empirical results from the latest benchmark run.
func (m *Monitor) SetBenchmarkSummary(throughputMBps, opsPerSec float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.latestThroughputMBps = throughputMBps
	m.latestOpsPerSec = opsPerSec
}

// RecordObservation logs an execution event into the monitor.
func (m *Monitor) RecordObservation(mode string, durationUs float64, bytesCount int, checksum uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.totalOps++
	if mode == "baseline_copy" || mode == "baseline" {
		m.baselineOps++
		if len(m.baselineLatenciesUs) < m.maxSamples/2 {
			m.baselineLatenciesUs = append(m.baselineLatenciesUs, durationUs)
		}
	} else {
		m.zeroCopyOps++
		if len(m.zeroCopyLatenciesUs) < m.maxSamples/2 {
			m.zeroCopyLatenciesUs = append(m.zeroCopyLatenciesUs, durationUs)
		}
	}
	m.bytesProcessed += uint64(bytesCount)
	m.lastChecksum = checksum

	if len(m.latenciesUs) < m.maxSamples {
		m.latenciesUs = append(m.latenciesUs, durationUs)
	} else {
		m.latenciesUs[m.sampleIndex] = durationUs
		m.sampleIndex = (m.sampleIndex + 1) % m.maxSamples
	}
}

// Snapshot computes non-parametric statistics and returns a frozen snapshot.
func (m *Monitor) Snapshot() MetricsSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	snap := MetricsSnapshot{
		Timestamp:         time.Now(),
		TotalOps:          m.totalOps,
		BaselineOps:       m.baselineOps,
		ZeroCopyOps:       m.zeroCopyOps,
		BytesProcessed:    m.bytesProcessed,
		LastChecksum:      m.lastChecksum,
		HeapAllocBytes:    mem.Alloc,
		HeapAllocMB:       float64(mem.Alloc) / (1024 * 1024),
		SysMemBytes:       mem.Sys,
		SysMemMB:          float64(mem.Sys) / (1024 * 1024),
		NumGC:             mem.NumGC,
		EdgeMemoryLimitMB: 512.0,
		MemoryPressurePct: (float64(mem.Sys) / (512 * 1024 * 1024)) * 100.0,
	}

	if m.latestOpsPerSec > 0 {
		snap.OpsPerSec = m.latestOpsPerSec
		snap.ThroughputMBps = m.latestThroughputMBps
	} else {
		elapsed := time.Since(m.startTime).Seconds()
		if elapsed > 0 {
			snap.OpsPerSec = float64(m.totalOps) / elapsed
			snap.ThroughputMBps = (float64(m.bytesProcessed) / (1024 * 1024)) / elapsed
		}
	}

	// Prefer zero-copy distribution for top metrics if present, else general pool
	targetLatencies := m.zeroCopyLatenciesUs
	if len(targetLatencies) == 0 {
		targetLatencies = m.latenciesUs
	}

	n := len(targetLatencies)
	if n == 0 {
		return snap
	}

	sorted := make([]float64, n)
	copy(sorted, targetLatencies)
	sort.Float64s(sorted)

	snap.MinLatencyUs = sorted[0]
	snap.MaxLatencyUs = sorted[n-1]
	snap.P50LatencyUs = percentile(sorted, 50.0)
	snap.P90LatencyUs = percentile(sorted, 90.0)
	snap.P95LatencyUs = percentile(sorted, 95.0)
	snap.P99LatencyUs = percentile(sorted, 99.0)

	var sum float64
	for _, v := range sorted {
		sum += v
	}
	snap.MeanLatencyUs = sum / float64(n)

	return snap
}

// Reset clears the metrics counters and sample buffer.
func (m *Monitor) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.startTime = time.Now()
	m.totalOps = 0
	m.baselineOps = 0
	m.zeroCopyOps = 0
	m.bytesProcessed = 0
	m.lastChecksum = 0
	m.latenciesUs = m.latenciesUs[:0]
	m.baselineLatenciesUs = m.baselineLatenciesUs[:0]
	m.zeroCopyLatenciesUs = m.zeroCopyLatenciesUs[:0]
	m.latestThroughputMBps = 0
	m.latestOpsPerSec = 0
	m.sampleIndex = 0
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 100 {
		return sorted[len(sorted)-1]
	}

	rank := (p / 100.0) * float64(len(sorted)-1)
	lower := int(math.Floor(rank))
	upper := int(math.Ceil(rank))
	weight := rank - float64(lower)

	if lower == upper {
		return sorted[lower]
	}
	return sorted[lower]*(1.0-weight) + sorted[upper]*weight
}
