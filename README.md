# VXR-Thesis

Evaluating Zero-Copy Execution and Custom Memory Allocation for Ultra-Low Latency WebAssembly Workloads.

[![CI Pipeline](https://github.com/Voxion-Labs/VXR-Thesis/actions/workflows/ci.yml/badge.svg)](https://github.com/Voxion-Labs/VXR-Thesis/actions/workflows/ci.yml)
[![Commit Manifest Bot](https://github.com/Voxion-Labs/VXR-Thesis/actions/workflows/commit_manifest.yml/badge.svg)](https://github.com/Voxion-Labs/VXR-Thesis/actions/workflows/commit_manifest.yml)

---

## Architecture Overview

VXR-Thesis explores architectural patterns for minimizing WebAssembly boundary serialization penalties:
- **C++ Core (`core-cpp/`)**: Custom zero-overhead memory allocator, direct memory buffer exchange, and Emscripten/WASI bindings.
- **Go Orchestrator (`cmd/vxr-server/`, `internal/`)**: High-performance HTTP server rendering dynamic partials via HTMX with embedded Wasm runtime execution and telemetry logging.
- **Python Empirical Analysis (`benchmarks/`)**: Benchmark suites measuring p50, p95, and p99 latency distributions, memory isolation profiles, and throughput scaling.
- **Academic Thesis (`thesis/`)**: LaTeX source files detailing hypotheses, formal methodology, and experimental results.

---

## CI/CD and Automation Pipelines

The repository includes a modern GitHub Actions pipeline:

| Workflow | Path | Description |
|---|---|---|
| **CI Pipeline** | `.github/workflows/ci.yml` | Validates Go (`vet`, `test`), Python syntax/bytecode compilation, C++ CMake builds, and manifest generation tooling on every push & pull request. |
| **Commit Manifest Bot** | `.github/workflows/commit_manifest.yml` | Automatically logs complete commit metadata to JSON records on `main`. |
| **Wasm Kernel Build** | `.github/workflows/build_wasm.yml` | Compiles C++ to WebAssembly artifacts when `core-cpp/` modifications occur. |
| **Render Deployment** | `.github/workflows/deploy_render.yml` | Automated webhook trigger for zero-downtime service deployment to Render. |

---

## Commit Manifest Bot

Every commit pushed to `main` is cataloged by the **Commit Manifest Bot** into structured JSON data:

- **Master Index (`manifests/manifest.json`)**:
  Chronological index containing all commit hashes, authors, timestamps, diff summaries, and links to detailed manifest files.
- **Per-Commit Records (`manifests/commits/<sha>.json`)**:
  Comprehensive JSON metadata for each individual commit:
  - Commit SHA, Short SHA, Tree SHA, Parent SHAs
  - Author and Committer identities with ISO-8601 timestamps
  - Full commit subject, body, and raw message
  - Diff statistics (files changed, insertions, deletions)
  - Detailed file status changes (`A`dded, `M`odified, `D`eleted, `R`enamed)
  - Bot recording timestamp and generator attribution

### Generating Manifests Locally
You can test or backfill the manifest locally using the Makefile:
```bash
# Generate manifest for the current HEAD commit
make manifest

# Backfill JSON manifests for all commits across git history
make manifest-backfill
```

---

## Write Permission & Access Control Policy

To ensure complete repository integrity, write permissions are guarded:
1. **GitHub Actions Scoping**:
   - The Manifest Bot runs **only on `push` to `main`**.
   - Pull requests from external forks are executed in an isolated, read-only token context (`permissions: contents: read`).
   - Fork PRs cannot execute write jobs or push commits.
2. **Actor Gating**:
   - The bot workflow enforces an actor check restricting write operations strictly to repository maintainers (`liambrooks-lab`, `R-Jena`) and `github-actions[bot]`.
3. **Loop Immunity**:
   - All bot commits append `[skip ci]` to prevent cascading execution cycles.

For full details on repository branch protection and ruleset setup, refer to [.github/SECURITY_PERMISSIONS.md](.github/SECURITY_PERMISSIONS.md).

---

## Developer Workflow & Makefile

A cross-platform `Makefile` is provided to streamline development, rebasing, and pushing changes:

```bash
# Show all available make targets
make help

# Stage all files, commit in a single commit, rebase origin/main, and push to GitHub
make push MSG="feat: your commit message"
# Alternatively:
make push m="feat: your commit message"

# Fetch from origin/main and rebase local branch with autostash
make rebase

# Sync local commits after rebasing origin/main
make sync

# Run all verification tests (Go vet, Go test, Python check)
make test

# Start the Go web server
make run-server

# Run Python latency benchmarks
make analyze
```

---

## Project Structure

```text
VXR-Thesis/
├── .github/
│   ├── SECURITY_PERMISSIONS.md     # Permission model and ruleset documentation
│   └── workflows/
│       ├── build_wasm.yml          # WebAssembly build workflow
│       ├── ci.yml                  # CI pipeline (Go, Python, C++)
│       ├── commit_manifest.yml     # Commit Manifest Bot workflow
│       └── deploy_render.yml       # Render continuous deployment hook
├── benchmarks/
│   ├── data/                       # Raw telemetry output (gitkeep preserved)
│   ├── plots/                      # Generated latency & memory graphs
│   ├── scripts/                    # Latency & memory isolation scripts
│   └── requirements.txt            # Python dependencies
├── cmd/
│   └── vxr-server/                 # Go entrypoint for HTMX orchestrator
├── core-cpp/
│   ├── CMakeLists.txt              # CMake build configuration
│   ├── include/                    # Kernel and zero-copy headers
│   ├── scripts/                    # WASM compilation script
│   └── src/                        # Kernel and allocator implementations
├── internal/
│   ├── engine/                     # Wasm runtime & executor
│   ├── telemetry/                  # Metrics monitoring & CSV export
│   └── web/                        # HTMX handlers & router
├── manifests/
│   ├── commits/                    # Individual commit JSON data files
│   └── manifest.json               # Master manifest index
├── scripts/
│   └── generate_manifest.py        # Commit manifest generation tool
├── thesis/                         # Academic paper LaTeX sources & hypothesis
├── web/                            # Templates and static assets
├── Dockerfile                      # Container definition for server
├── docker-compose.yml              # Local orchestration
├── go.mod                          # Go module definition
├── Makefile                        # Automation recipes and git helper
└── README.md                       # Project documentation
```

---

## License

This project is licensed under the terms specified in the [LICENSE](LICENSE) file.
