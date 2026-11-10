#!/usr/bin/env python3
"""
VXR-Thesis: Automated Systems Benchmark Suite Runner
Executes the Go orchestrator across the full empirical matrix and triggers
generation of publication-grade figures.
"""

import os
import subprocess
import sys


def run_cmd(cmd, cwd=None):
    print(f"[EXEC] {' '.join(cmd)}")
    res = subprocess.run(cmd, cwd=cwd, text=True, capture_output=True)
    if res.returncode != 0:
        print(f"[ERROR] Command failed:\n{res.stderr}", file=sys.stderr)
        sys.exit(res.returncode)
    return res.stdout


def main():
    root_dir = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
    csv_path = os.path.join(root_dir, "benchmarks", "data", "telemetry.csv")
    plots_dir = os.path.join(root_dir, "benchmarks", "plots")
    os.makedirs(os.path.dirname(csv_path), exist_ok=True)
    os.makedirs(plots_dir, exist_ok=True)

    print("================================================================================")
    print("  VXR-Thesis: Automated Empirical Systems Benchmarking Pipeline                 ")
    print("================================================================================")

    # 1. Parameter Matrix
    concurrency_levels = [1, 4, 8]
    payload_sizes = [64, 512, 4096, 65536]
    iterations_per_run = 1000

    print(f"[INFO] Matrix: {len(concurrency_levels)} concurrency levels x {len(payload_sizes)} payload sizes")
    print(f"[INFO] Total Conditions: {len(concurrency_levels) * len(payload_sizes) * 2} runs (dual-path)")

    for c in concurrency_levels:
        for p in payload_sizes:
            print(f"\n--> Running condition: Concurrency={c}, Payload={p} bytes, Iterations={iterations_per_run}")
            run_cmd([
                "go", "run", "cmd/vxr-server/main.go",
                "-mode", "both",
                "-concurrency", str(c),
                "-iterations", str(iterations_per_run),
                "-payload", str(p),
                "-csv", csv_path,
            ], cwd=root_dir)

    print("\n================================================================================")
    print("  Executing Statistical Evaluators & Vector Plot Generators                     ")
    print("================================================================================")

    # 2. Run Latency & Throughput Analyzer
    run_cmd([sys.executable, os.path.join(root_dir, "benchmarks", "scripts", "analyze_latency.py")], cwd=root_dir)

    # 3. Run Memory Isolation Visualizer
    run_cmd([sys.executable, os.path.join(root_dir, "benchmarks", "scripts", "plot_memory_isolation.py")], cwd=root_dir)

    # 4. Verify Generated Plots
    expected_files = [
        "latency_cdf.png", "latency_cdf.pdf",
        "tail_latency_concurrency.png", "tail_latency_concurrency.pdf",
        "throughput_scaling.png", "throughput_scaling.pdf",
        "memory_isolation_timeline.png", "memory_isolation_timeline.pdf",
    ]

    print("\n[VERIFICATION OF GENERATED ARTIFACTS]")
    all_ok = True
    for f in expected_files:
        p = os.path.join(plots_dir, f)
        if os.path.exists(p) and os.path.getsize(p) > 0:
            print(f"  [OK] {f} ({os.path.getsize(p)} bytes)")
        else:
            print(f"  [FAIL] Missing {f}")
            all_ok = False

    if not all_ok:
        sys.exit(1)

    print("\n================================================================================")
    print("  All Empirical Benchmark Visualizations Successfully Generated!                ")
    print("================================================================================")


if __name__ == "__main__":
    main()
