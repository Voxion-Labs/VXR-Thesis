package engine_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/R-Jena/VXR-Thesis/internal/engine"
)

func getWasmPath(t *testing.T) string {
	path, err := filepath.Abs("../../web/static/wasm/vxr_kernel.wasm")
	if err != nil {
		t.Fatalf("failed to resolve wasm path: %v", err)
	}
	return path
}

func TestLoadWasmKernel(t *testing.T) {
	ctx := context.Background()
	wasmPath := getWasmPath(t)

	instance, err := engine.LoadWasmKernel(ctx, wasmPath)
	if err != nil {
		t.Fatalf("failed to load wasm kernel: %v", err)
	}
	defer instance.Close(ctx)

	if instance.RingBufferPtr() == 0 {
		t.Errorf("expected non-zero ring buffer pointer, got %d", instance.RingBufferPtr())
	}

	if instance.RingCapacity() == 0 {
		t.Errorf("expected non-zero ring capacity, got %d", instance.RingCapacity())
	}
}

func TestDualPathEquivalence(t *testing.T) {
	ctx := context.Background()
	wasmPath := getWasmPath(t)

	instance, err := engine.LoadWasmKernel(ctx, wasmPath)
	if err != nil {
		t.Fatalf("failed to load wasm kernel: %v", err)
	}
	defer instance.Close(ctx)

	executor := engine.NewExecutor(instance)

	payloadSize := 4096
	payload := make([]byte, payloadSize)
	for i := range payload {
		payload[i] = byte((i*37 + 11) & 0xFF)
	}

	// 1. Execute Baseline Copy Path
	copyResult, err := executor.ExecuteCopy(ctx, payload)
	if err != nil {
		t.Fatalf("ExecuteCopy failed: %v", err)
	}

	// 2. Execute Zero-Copy Path
	zeroResult, err := executor.ExecuteZeroCopy(ctx, 0, payload)
	if err != nil {
		t.Fatalf("ExecuteZeroCopy failed: %v", err)
	}

	// Mathematical equivalence check
	if copyResult.Checksum != zeroResult.Checksum {
		t.Errorf("checksum mismatch: copy=0x%X, zero_copy=0x%X", copyResult.Checksum, zeroResult.Checksum)
	}

	if copyResult.BytesProcessed != payloadSize || zeroResult.BytesProcessed != payloadSize {
		t.Errorf("bytes processed mismatch: copy=%d, zero=%d, expected=%d",
			copyResult.BytesProcessed, zeroResult.BytesProcessed, payloadSize)
	}

	t.Logf("Both paths produced identical cryptographic checksum: 0x%X", copyResult.Checksum)
	t.Logf("Baseline Copy latency: %.3f µs, Zero-Copy latency: %.3f µs",
		copyResult.DurationUs, zeroResult.DurationUs)
}
