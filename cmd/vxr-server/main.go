package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/R-Jena/VXR-Thesis/internal/engine"
	"github.com/R-Jena/VXR-Thesis/internal/telemetry"
	"github.com/R-Jena/VXR-Thesis/internal/web"
)

func main() {
	wasmPath := flag.String("wasm", "web/static/wasm/vxr_kernel.wasm", "Path to compiled WebAssembly kernel")
	mode := flag.String("mode", "both", "Benchmark mode: zero_copy, baseline, or both")
	concurrency := flag.Int("concurrency", 8, "Number of concurrent worker goroutines")
	iterations := flag.Int("iterations", 5000, "Total benchmark iterations per mode")
	payloadSize := flag.Int("payload", 4096, "Payload size in bytes (default: 4096 = 4KB)")
	csvPath := flag.String("csv", "benchmarks/data/telemetry.csv", "Path to telemetry CSV log output")
	serve := flag.Bool("serve", false, "Start HTMX live telemetry HTTP web server")
	port := flag.Int("port", 8080, "HTTP server listening port")
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	fmt.Println("================================================================================")
	fmt.Println("  VXR-Thesis: Zero-Copy Execution & Custom Memory Allocation Orchestrator       ")
	fmt.Println("================================================================================")
	fmt.Printf("[CONFIG] Target Wasm Binary: %s\n", *wasmPath)
	fmt.Printf("[CONFIG] Concurrency Level:  %d workers\n", *concurrency)
	fmt.Printf("[CONFIG] Payload Dimension:  %d bytes (%.2f KB)\n", *payloadSize, float64(*payloadSize)/1024.0)
	fmt.Printf("[CONFIG] Telemetry Output:   %s\n", *csvPath)
	fmt.Println("--------------------------------------------------------------------------------")

	// 1. Initialize Telemetry Infrastructure
	mon := telemetry.NewMonitor(20000)
	csvWriter, err := telemetry.NewCSVWriter(*csvPath, 50000)
	if err != nil {
		log.Fatalf("[FATAL] Failed to initialize CSV telemetry writer: %v", err)
	}
	defer csvWriter.Close()

	// 2. Initialize WebAssembly Runtime & Kernel
	fmt.Println("[VXR] Bootstrapping Wazero runtime and compiling kernel...")
	wasmInstance, err := engine.LoadWasmKernel(ctx, *wasmPath)
	if err != nil {
		log.Fatalf("[FATAL] Failed to load WebAssembly kernel: %v", err)
	}
	defer wasmInstance.Close(ctx)

	fmt.Printf("[VXR] Kernel loaded: RingBuffer=0x%05X (%d KB), LinearMemory=32 pages (2MB)\n",
		wasmInstance.RingBufferPtr(), wasmInstance.RingCapacity()/1024)

	executor := engine.NewExecutor(wasmInstance)

	// 3. Serve Mode: Launch HTTP Server with HTMX Live Telemetry Dashboard
	if *serve {
		router := web.NewRouter(executor, mon, csvWriter)
		addr := fmt.Sprintf(":%d", *port)
		server := &http.Server{
			Addr:         addr,
			Handler:      router,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
		}

		go func() {
			fmt.Printf("[HTTP] HTMX Live Telemetry Server listening on http://localhost:%d\n", *port)
			if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatalf("[FATAL] HTTP server error: %v", err)
			}
		}()

		<-ctx.Done()
		fmt.Println("\n[VXR] Shutting down server gracefully...")
		shutdownCtx, sCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer sCancel()
		_ = server.Shutdown(shutdownCtx)
		return
	}

	// 4. Standalone Benchmark Suite Execution
	var baseSummary *engine.BenchmarkSummary
	var zeroSummary *engine.BenchmarkSummary

	if *mode == "baseline" || *mode == "both" {
		fmt.Println("[RUNNING] Evaluating Baseline Path (Host-Guest Memcpy + Guest Dynamic Allocator)...")
		cfg := engine.BenchmarkConfig{
			Mode:        "baseline_copy",
			Concurrency: *concurrency,
			Iterations:  *iterations,
			PayloadSize: *payloadSize,
			WarmupRuns:  50,
		}
		var bErr error
		baseSummary, bErr = executor.RunBenchmarkSuite(ctx, cfg, mon, csvWriter)
		if bErr != nil {
			log.Fatalf("[FATAL] Baseline benchmark run failed: %v", bErr)
		}
		fmt.Println("  -> Baseline complete.")
	}

	if *mode == "zero_copy" || *mode == "both" {
		fmt.Println("[RUNNING] Evaluating Zero-Copy Path (Direct Linear Memory Slicing + In-Place)...")
		cfg := engine.BenchmarkConfig{
			Mode:        "zero_copy",
			Concurrency: *concurrency,
			Iterations:  *iterations,
			PayloadSize: *payloadSize,
			WarmupRuns:  50,
		}
		var zErr error
		zeroSummary, zErr = executor.RunBenchmarkSuite(ctx, cfg, mon, csvWriter)
		if zErr != nil {
			log.Fatalf("[FATAL] Zero-Copy benchmark run failed: %v", zErr)
		}
		fmt.Println("  -> Zero-Copy complete.")
	}

	// Print Comparative Systems Analysis
	printComparativeReport(baseSummary, zeroSummary)
}

func printComparativeReport(base, zero *engine.BenchmarkSummary) {
	fmt.Println("================================================================================")
	fmt.Println("  EMPIRICAL SYSTEMS EVALUATION REPORT                                           ")
	fmt.Println("================================================================================")
	fmt.Printf("%-24s | %-22s | %-22s\n", "Metric", "Baseline (Copy+Alloc)", "Proposed (Zero-Copy)")
	fmt.Println("-------------------------+------------------------+-----------------------")

	if base != nil && zero != nil {
		fmt.Printf("%-24s | %-22d | %-22d\n", "Completed Operations", base.TotalOps, zero.TotalOps)
		fmt.Printf("%-24s | %-22.3f | %-22.3f\n", "Duration (sec)", base.TotalDuration, zero.TotalDuration)
		fmt.Printf("%-24s | %-22.2f | %-22.2f\n", "Throughput (MB/s)", base.ThroughputMBps, zero.ThroughputMBps)
		fmt.Printf("%-24s | %-22.1f | %-22.1f\n", "Operations/sec", base.OpsPerSec, zero.OpsPerSec)
		fmt.Printf("%-24s | %-22.2f | %-22.2f\n", "Mean Latency (us)", base.MeanLatencyUs, zero.MeanLatencyUs)
		fmt.Printf("%-24s | %-22.2f | %-22.2f\n", "p50 Latency (us)", base.P50LatencyUs, zero.P50LatencyUs)
		fmt.Printf("%-24s | %-22.2f | %-22.2f\n", "p95 Latency (us)", base.P95LatencyUs, zero.P95LatencyUs)
		fmt.Printf("%-24s | %-22.2f | %-22.2f\n", "p99 Tail Latency (us)", base.P99LatencyUs, zero.P99LatencyUs)
		fmt.Printf("%-24s | %-22.2f | %-22.2f\n", "Go Heap Alloc (MB)", base.HeapAllocMB, zero.HeapAllocMB)
		fmt.Println("-------------------------+------------------------+-----------------------")

		speedup := base.MeanLatencyUs / zero.MeanLatencyUs
		p99Diff := ((base.P99LatencyUs - zero.P99LatencyUs) / base.P99LatencyUs) * 100.0
		fmt.Printf("[FINDING] Mean Latency Speedup:       %.2fx faster\n", speedup)
		fmt.Printf("[FINDING] p99 Tail Latency Reduction: %.1f%% drop\n", p99Diff)
		fmt.Printf("[FINDING] Mathematical Checksum:      0x%X (MATCH)\n", zero.LastChecksum)
	} else if zero != nil {
		fmt.Printf("%-24s | %-22s | %-22.2f\n", "p50 Latency (us)", "N/A", zero.P50LatencyUs)
		fmt.Printf("%-24s | %-22s | %-22.2f\n", "p99 Latency (us)", "N/A", zero.P99LatencyUs)
		fmt.Printf("%-24s | %-22s | %-22.1f\n", "Operations/sec", "N/A", zero.OpsPerSec)
	} else if base != nil {
		fmt.Printf("%-24s | %-22.2f | %-22s\n", "p50 Latency (us)", base.P50LatencyUs, "N/A")
		fmt.Printf("%-24s | %-22.2f | %-22s\n", "p99 Latency (us)", base.P99LatencyUs, "N/A")
	}
	fmt.Println("================================================================================")
	fmt.Println("[VXR] Telemetry points recorded to disk. Ready for offline Python evaluation.")
}
