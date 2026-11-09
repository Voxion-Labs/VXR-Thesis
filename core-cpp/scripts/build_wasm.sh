#!/usr/bin/env bash
# ==============================================================================
# VXR-Thesis: Build C++ Kernel to Standalone WebAssembly
# ==============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
OUTPUT_DIR="${ROOT_DIR}/web/static/wasm"
OUTPUT_WASM="${OUTPUT_DIR}/vxr_kernel.wasm"

echo "==> [VXR] WebAssembly Kernel Build"

if ! command -v emcc &> /dev/null; then
    echo "[VXR-WARN] 'emcc' (Emscripten) not found in PATH."
    echo "           In local environments without emsdk, native compilation and precompiled artifacts are used."
    echo "           Full Emscripten compilation runs automatically in GitHub Actions CI."
    exit 0
fi

mkdir -p "${OUTPUT_DIR}"

echo "==> Compiling C++ kernel with emcc..."
emcc -O3 -flto \
    -std=c++17 \
    -I "${ROOT_DIR}/core-cpp/include" \
    "${ROOT_DIR}/core-cpp/src/custom_allocator.cpp" \
    "${ROOT_DIR}/core-cpp/src/zero_copy_stream.cpp" \
    "${ROOT_DIR}/core-cpp/src/vxr_kernel.cpp" \
    "${ROOT_DIR}/core-cpp/src/bindings.cpp" \
    -s STANDALONE_WASM=1 \
    -s EXPORTED_FUNCTIONS="['_vxr_init', '_vxr_get_ring_buffer_ptr', '_vxr_get_ring_buffer_capacity', '_vxr_alloc_arena', '_vxr_reset_arena', '_vxr_alloc_slab', '_vxr_free_slab', '_vxr_process_copy', '_vxr_process_zero_copy', '_vxr_compute_checksum', '_vxr_get_stats_total_ops', '_vxr_get_stats_copy_ops', '_vxr_get_stats_zero_copy_ops', '_vxr_get_stats_bytes_processed', '_vxr_get_stats_last_checksum']" \
    -s ERROR_ON_UNDEFINED_SYMBOLS=0 \
    -s ALLOW_MEMORY_GROWTH=1 \
    --no-entry \
    -o "${OUTPUT_WASM}"

echo "==> Successfully generated: ${OUTPUT_WASM}"
ls -lh "${OUTPUT_WASM}"
