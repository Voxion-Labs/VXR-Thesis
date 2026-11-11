package web_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/R-Jena/VXR-Thesis/internal/engine"
	"github.com/R-Jena/VXR-Thesis/internal/telemetry"
	"github.com/R-Jena/VXR-Thesis/internal/web"
)

func setupTestServer(t *testing.T) http.Handler {
	// Navigate to project root for template resolution if needed
	wd, _ := os.Getwd()
	if filepath.Base(wd) == "web" {
		_ = os.Chdir("../..")
	}

	ctx := context.Background()
	wasmPath := "web/static/wasm/vxr_kernel.wasm"

	instance, err := engine.LoadWasmKernel(ctx, wasmPath)
	if err != nil {
		t.Fatalf("failed to load wasm kernel: %v", err)
	}

	executor := engine.NewExecutor(instance)
	monitor := telemetry.NewMonitor(100)
	writer, err := telemetry.NewCSVWriter("benchmarks/data/test_telemetry.csv", 100)
	if err != nil {
		t.Fatalf("failed to create CSV writer: %v", err)
	}

	t.Cleanup(func() {
		_ = os.Remove("benchmarks/data/test_telemetry.csv")
	})

	return web.NewRouter(executor, monitor, writer)
}

func TestWebEndpoints(t *testing.T) {
	router := setupTestServer(t)

	// 1. Test Index HTML
	reqIndex := httptest.NewRequest(http.MethodGet, "/", nil)
	recIndex := httptest.NewRecorder()
	router.ServeHTTP(recIndex, reqIndex)

	if recIndex.Code != http.StatusOK {
		t.Errorf("expected status 200 for index, got %d", recIndex.Code)
	}
	if !strings.Contains(recIndex.Body.String(), "VXR-Thesis") {
		t.Errorf("expected index to contain 'VXR-Thesis'")
	}

	// 2. Test Latency Partial
	reqLatency := httptest.NewRequest(http.MethodGet, "/api/metrics/latency", nil)
	recLatency := httptest.NewRecorder()
	router.ServeHTTP(recLatency, reqLatency)

	if recLatency.Code != http.StatusOK {
		t.Errorf("expected status 200 for latency partial, got %d", recLatency.Code)
	}
	if !strings.Contains(recLatency.Body.String(), "p50 Median Latency") {
		t.Errorf("expected latency partial to contain 'p50 Median Latency'")
	}

	// 3. Test Memory Partial
	reqMem := httptest.NewRequest(http.MethodGet, "/api/metrics/memory", nil)
	recMem := httptest.NewRecorder()
	router.ServeHTTP(recMem, reqMem)

	if recMem.Code != http.StatusOK {
		t.Errorf("expected status 200 for memory partial, got %d", recMem.Code)
	}
	if !strings.Contains(recMem.Body.String(), "Edge Memory Utilization") {
		t.Errorf("expected memory partial to contain 'Edge Memory Utilization'")
	}

	// 4. Test Trigger Benchmark
	formData := url.Values{
		"mode":        {"both"},
		"concurrency": {"2"},
		"iterations":  {"100"},
		"payload":     {"1024"},
	}
	reqTrigger := httptest.NewRequest(http.MethodPost, "/api/benchmark/trigger", strings.NewReader(formData.Encode()))
	reqTrigger.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recTrigger := httptest.NewRecorder()
	router.ServeHTTP(recTrigger, reqTrigger)

	if recTrigger.Code != http.StatusOK {
		t.Errorf("expected status 200 for trigger, got %d", recTrigger.Code)
	}
	if !strings.Contains(recTrigger.Body.String(), "Workload Finished") {
		t.Errorf("expected output to contain 'Workload Finished', got: %s", recTrigger.Body.String())
	}

	// 4b. Test Trigger with Custom Parameter Inputs
	formCustom := url.Values{
		"mode":               {"both"},
		"concurrency":        {"4"},
		"custom_concurrency": {"3"},
		"payload":            {"4096"},
		"custom_payload":     {"2048"},
		"iterations":         {"5000"},
		"custom_iterations":  {"150"},
	}
	reqCustom := httptest.NewRequest(http.MethodPost, "/api/benchmark/trigger", strings.NewReader(formCustom.Encode()))
	reqCustom.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recCustom := httptest.NewRecorder()
	router.ServeHTTP(recCustom, reqCustom)

	if recCustom.Code != http.StatusOK {
		t.Errorf("expected status 200 for custom trigger, got %d", recCustom.Code)
	}
	if !strings.Contains(recCustom.Body.String(), "Workers: 3") || !strings.Contains(recCustom.Body.String(), "2048 B") {
		t.Errorf("expected custom parameters to be parsed and executed, got: %s", recCustom.Body.String())
	}

	// 5. Test Cloud Health Endpoints (/health and /healthz)
	reqHealth := httptest.NewRequest(http.MethodGet, "/health", nil)
	recHealth := httptest.NewRecorder()
	router.ServeHTTP(recHealth, reqHealth)
	if recHealth.Code != http.StatusOK || recHealth.Body.String() != "OK" {
		t.Errorf("expected 200 OK from /health, got code %d body %s", recHealth.Code, recHealth.Body.String())
	}

	reqHealthz := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	recHealthz := httptest.NewRecorder()
	router.ServeHTTP(recHealthz, reqHealthz)
	if recHealthz.Code != http.StatusOK || recHealthz.Body.String() != "OK" {
		t.Errorf("expected 200 OK from /healthz, got code %d body %s", recHealthz.Code, recHealthz.Body.String())
	}
}
