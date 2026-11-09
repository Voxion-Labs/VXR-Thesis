#pragma once

#include "custom_allocator.hpp"
#include "zero_copy_stream.hpp"
#include <cstddef>
#include <cstdint>

namespace vxr {

struct KernelMetrics {
    uint64_t total_ops;
    uint64_t copy_ops;
    uint64_t zero_copy_ops;
    uint64_t bytes_processed;
    uint64_t last_checksum;
    uint32_t arena_used_bytes;
    uint32_t slab_free_chunks;
};

/**
 * @brief VXRKernel
 * 
 * Central execution sandbox managing linear memory regions, allocators,
 * and dual-path compute execution for latency & throughput benchmarking.
 */
class VXRKernel {
public:
    VXRKernel();
    ~VXRKernel();

    // Initialize kernel memory layout
    bool initialize(size_t ring_buffer_size = 256 * 1024,
                    size_t arena_size = 512 * 1024,
                    size_t slab_size = 256 * 1024,
                    size_t slab_chunk_size = 256);

    // Path 1: Traditional Host-Guest Copy Path (malloc + memcpy + compute + memcpy + free)
    uint64_t execute_copy_path(const uint8_t* host_data, size_t len, uint8_t* out_buf);

    // Path 2: Zero-Copy In-Place Path (direct linear memory computation)
    uint64_t execute_zero_copy_path(size_t offset, size_t len);

    // Direct accessors
    ZeroCopyRingBuffer& get_ring_buffer() { return ring_buffer_; }
    ArenaAllocator& get_arena() { return arena_; }
    SlabAllocator& get_slab() { return slab_; }
    const KernelMetrics& get_metrics() const { return metrics_; }

    // Fast 64-bit FNV-1a non-cryptographic checksum
    static uint64_t compute_checksum(const uint8_t* data, size_t len);

    // In-place streaming payload transformation
    static void transform_payload(uint8_t* data, size_t len, uint64_t seed);

private:
    uint8_t* memory_pool_ = nullptr;
    size_t total_pool_size_ = 0;

    ZeroCopyRingBuffer ring_buffer_;
    ArenaAllocator arena_;
    SlabAllocator slab_;

    KernelMetrics metrics_ = {0};
    bool initialized_ = false;
};

// Global kernel instance for C-ABI exports
VXRKernel& get_global_kernel();

} // namespace vxr
