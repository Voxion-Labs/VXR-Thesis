# ==============================================================================
# VXR-Thesis: Makefile
# Zero-Copy Execution and Custom Memory Allocation for Ultra-Low Latency Wasm
# ==============================================================================

BRANCH  ?= main
REMOTE  ?= origin

# Convenient commit message: accepts MSG="..." or m="..."
# Defaults to a clean automated update message if not provided.
MSG ?= $(m)
ifeq ($(strip $(MSG)),)
  COMMIT_MSG := update: automated workspace sync
else
  COMMIT_MSG := $(MSG)
endif

.PHONY: help push rebase sync manifest manifest-backfill build-wasm verify-wasm verify-thesis run-server benchmark analyze test test-cpp status

# Default target
all: help

# ------------------------------------------------------------------------------
# Git & CI/CD Workflow Targets
# ------------------------------------------------------------------------------

## push: Stage all files, commit in a single commit, rebase origin/main, and push to GitHub
## Usage: make push MSG="your commit message"
##    or: make push m="your commit message"
##    or: make push (uses default automated message)
push:
	@echo [VXR] Staging all workspace changes...
	git add -A
	@echo [VXR] Committing all changes in a single commit...
	@git diff --cached --quiet || git commit -m "$(COMMIT_MSG)"
	@echo [VXR] Fetching $(REMOTE)/$(BRANCH)...
	git fetch $(REMOTE) $(BRANCH)
	@echo [VXR] Rebasing local branch onto $(REMOTE)/$(BRANCH)...
	git rebase --autostash $(REMOTE)/$(BRANCH)
	@echo [VXR] Pushing to $(REMOTE) $(BRANCH)...
	git push $(REMOTE) $(BRANCH)
	@echo [VXR] Successfully pushed to $(REMOTE)/$(BRANCH) in a single commit!

## rebase: Fetch from origin main and rebase current branch
rebase:
	@echo [VXR] Fetching latest changes from $(REMOTE) $(BRANCH)...
	git fetch $(REMOTE) $(BRANCH)
	@echo [VXR] Rebasing onto $(REMOTE)/$(BRANCH)...
	git rebase --autostash $(REMOTE)/$(BRANCH)
	@echo [VXR] Rebase completed successfully.

## sync: Rebase onto origin main and push existing local commits
sync: rebase
	@echo [VXR] Pushing current branch to $(REMOTE) $(BRANCH)...
	git push $(REMOTE) $(BRANCH)

## manifest: Run the commit manifest generator locally for HEAD
manifest:
	@echo [VXR] Generating commit manifest for HEAD...
	python scripts/generate_manifest.py --commit-sha HEAD

## manifest-backfill: Regenerate manifest for all commits in repository history
manifest-backfill:
	@echo [VXR] Backfilling commit manifest for entire git history...
	python scripts/generate_manifest.py --backfill

## status: Show short git status
status:
	@git status -s

# ------------------------------------------------------------------------------
# Build, Run, and Benchmark Targets
# ------------------------------------------------------------------------------

## build-wasm: Compile/synthesize C++ kernel to WebAssembly
build-wasm:
	@echo [VXR] Building VXR WebAssembly kernel...
	python scripts/compile_wasm_kernel.py
	python scripts/verify_wasm.py

## verify-wasm: Validate WebAssembly binary ABI compliance
verify-wasm:
	@echo [VXR] Validating WebAssembly binary ABI...
	python scripts/verify_wasm.py

## run-server: Start the Go orchestrator server
run-server:
	@echo [VXR] Starting VXR-Thesis server...
	go run cmd/vxr-server/main.go

## benchmark: Execute full empirical parameter matrix and generate publication plots
benchmark:
	@echo [VXR] Executing full systems benchmark matrix...
	python scripts/run_benchmarks.py

## analyze: Run empirical statistical analysis and generate vector plots
analyze:
	@echo [VXR] Running empirical statistical analysis and plot generators...
	python benchmarks/scripts/analyze_latency.py
	python benchmarks/scripts/plot_memory_isolation.py

## test-cpp: Compile and execute native C++ allocator and kernel verification tests
test-cpp:
	@echo [VXR] Compiling and running native C++ systems verification suite...
	@g++ -std=c++17 -O3 -Icore-cpp/include core-cpp/src/*.cpp core-cpp/tests/test_kernel.cpp -o test_kernel_run.exe && test_kernel_run.exe && python -c "import os; os.remove('test_kernel_run.exe') if os.path.exists('test_kernel_run.exe') else None"

## verify-thesis: Validate LaTeX thesis structure, citations, and figure paths
verify-thesis:
	@echo [VXR] Validating LaTeX academic thesis and citations...
	python scripts/verify_thesis.py

## test: Vet Go codebase, execute C++ tests, verify Wasm ABI, test thesis, and verify Python scripts
test: test-cpp verify-wasm verify-thesis
	@echo [VXR] Vetting Go code...
	go vet ./...
	@echo [VXR] Testing Go packages...
	go test ./...
	@echo [VXR] Compiling Python scripts...
	python -c "import glob, py_compile; [py_compile.compile(f, doraise=True) for p in ['benchmarks/scripts/*.py', 'scripts/*.py'] for f in glob.glob(p)]"
	@echo [VXR] All checks passed!

## help: Show this help message
help:
	@echo VXR-Thesis Build and Automation System
	@echo.
	@echo Usage: make [target] [MSG="commit message"]
	@echo.
	@echo Git and Deployment Targets:
	@echo   push               Stage all files, commit once, rebase origin/main, and push
	@echo                      Usage: make push MSG="feat: update architecture"
	@echo   rebase             Fetch origin/main and rebase local branch
	@echo   sync               Rebase origin/main and push commits to remote
	@echo   manifest           Generate/update commit JSON manifest for HEAD
	@echo   manifest-backfill  Backfill JSON manifests for all historical commits
	@echo   status             Display concise git status
	@echo.
	@echo Development Targets:
	@echo   build-wasm         Build C++ kernel into WebAssembly
	@echo   verify-wasm        Validate WebAssembly binary ABI compliance
	@echo   verify-thesis      Validate LaTeX thesis structure, citations, and figures
	@echo   run-server         Run Go orchestrator web server
	@echo   benchmark          Execute full empirical parameter matrix and generate plots
	@echo   analyze            Run empirical statistical analysis and generate vector plots
	@echo   test-cpp           Compile and execute native C++ allocator and kernel tests
	@echo   test               Run C++ tests, Go vet, Go tests, and Python syntax checks
	@echo   help               Display this help text
