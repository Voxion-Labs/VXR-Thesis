package telemetry_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/R-Jena/VXR-Thesis/internal/telemetry"
)

func TestMonitorPercentiles(t *testing.T) {
	mon := telemetry.NewMonitor(1000)

	// Ingest synthetic latencies 1..100 µs
	for i := 1; i <= 100; i++ {
		mon.RecordObservation("zero_copy", float64(i), 1024, 0x1234)
	}

	snap := mon.Snapshot()

	if snap.TotalOps != 100 {
		t.Errorf("expected 100 total ops, got %d", snap.TotalOps)
	}

	if snap.P50LatencyUs < 49.0 || snap.P50LatencyUs > 51.0 {
		t.Errorf("expected p50 around 50 µs, got %.2f", snap.P50LatencyUs)
	}

	if snap.P99LatencyUs < 98.0 || snap.P99LatencyUs > 100.0 {
		t.Errorf("expected p99 around 99 µs, got %.2f", snap.P99LatencyUs)
	}

	if snap.MinLatencyUs != 1.0 {
		t.Errorf("expected min 1.0, got %.2f", snap.MinLatencyUs)
	}

	if snap.MaxLatencyUs != 100.0 {
		t.Errorf("expected max 100.0, got %.2f", snap.MaxLatencyUs)
	}
}

func TestCSVWriter(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "vxr_telemetry_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	testCSV := filepath.Join(tmpDir, "test_telemetry.csv")
	writer, err := telemetry.NewCSVWriter(testCSV, 100)
	if err != nil {
		t.Fatalf("failed to create CSV writer: %v", err)
	}

	record := telemetry.LogRecord{
		Timestamp:      time.Now(),
		Mode:           "zero_copy",
		PayloadSize:    4096,
		Concurrency:    4,
		LatencyUs:      12.5,
		Checksum:       0xABCDEF,
		HeapAllocBytes: 1024 * 1024,
		SysMemBytes:    8 * 1024 * 1024,
		NumGC:          1,
	}

	writer.Log(record)
	if err := writer.Close(); err != nil {
		t.Fatalf("failed to close CSV writer: %v", err)
	}

	data, err := os.ReadFile(testCSV)
	if err != nil {
		t.Fatalf("failed to read written CSV: %v", err)
	}

	if len(data) == 0 {
		t.Errorf("expected non-empty CSV output")
	}
}
