#include "vxr_kernel.hpp"
#include <cstdlib>
#include <cstring>

namespace vxr {

// FNV-1a 64-bit constants
constexpr uint64_t FNV_OFFSET_BASIS = 14695981039346656037ULL;
constexpr uint64_t FNV_PRIME = 1099511628211ULL;

// Static linear memory arena for standalone Wasm environments (2 MB default)
constexpr size_t STATIC_WASM_POOL_SIZE = 2 * 1024 * 1024;
static uint8_t s_wasm_memory_pool[STATIC_WASM_POOL_SIZE] __attribute__((aligned(64)));

VXRKernel::VXRKernel() = default;

VXRKernel::~VXRKernel() {
    // If dynamically allocated outside static pool, cleanup would occur here
}

bool VXRKernel::initialize(size_t ring_buffer_size,
                           size_t arena_size,
                           size_t slab_size,
                           size_t slab_chunk_size) {
    total_pool_size_ = ring_buffer_size + arena_size + slab_size;
    if (total_pool_size_ > STATIC_WASM_POOL_SIZE) {
        return false;
    }

    memory_pool_ = s_wasm_memory_pool;
    std::memset(memory_pool_, 0, total_pool_size_);

    uint8_t* ptr = memory_pool_;

    // Partition 1: Zero-Copy Circular Ring Buffer
    ring_buffer_.init(ptr, ring_buffer_size);
    ptr += ring_buffer_size;

    // Partition 2: Monotonic Arena Allocator
    arena_.init(ptr, arena_size);
    ptr += arena_size;

    // Partition 3: O(1) Fixed-Chunk Slab Allocator
    slab_.init(ptr, slab_size, slab_chunk_size);

    std::memset(&metrics_, 0, sizeof(metrics_));
    initialized_ = true;
    return true;
}

uint64_t VXRKernel::compute_checksum(const uint8_t* data, size_t len) {
    uint64_t hash = FNV_OFFSET_BASIS;
    for (size_t i = 0; i < len; ++i) {
        hash ^= data[i];
        hash *= FNV_PRIME;
    }
    return hash;
}

void VXRKernel::transform_payload(uint8_t* data, size_t len, uint64_t seed) {
    uint8_t mask = static_cast<uint8_t>(seed & 0xFF);
    // Unrolled stream transformation
    size_t i = 0;
    for (; i + 4 <= len; i += 4) {
        data[i]     ^= mask;
        data[i + 1] ^= (mask + 1);
        data[i + 2] ^= (mask + 2);
        data[i + 3] ^= (mask + 3);
    }
    for (; i < len; ++i) {
        data[i] ^= (mask + static_cast<uint8_t>(i));
    }
}

// -----------------------------------------------------------------------------
// Path 1: Traditional Host-Guest Copy Path
// (Simulates overhead: Host Mem -> Guest Alloc -> Guest Copy -> Process -> Guest Copy Out -> Free)
// -----------------------------------------------------------------------------
uint64_t VXRKernel::execute_copy_path(const uint8_t* host_data, size_t len, uint8_t* out_buf) {
    if (!initialized_ || !host_data || len == 0) {
        return 0;
    }

    // Allocation overhead
    auto* guest_buffer = static_cast<uint8_t*>(std::malloc(len));
    if (!guest_buffer) {
        return 0;
    }

    // Boundary Copy 1: Host to Guest
    std::memcpy(guest_buffer, host_data, len);

    // Compute workload
    uint64_t initial_hash = compute_checksum(guest_buffer, len);
    transform_payload(guest_buffer, len, initial_hash);
    uint64_t final_hash = compute_checksum(guest_buffer, len);

    // Boundary Copy 2: Guest to Output Buffer
    if (out_buf) {
        std::memcpy(out_buf, guest_buffer, len);
    }

    // Deallocation overhead
    std::free(guest_buffer);

    metrics_.total_ops++;
    metrics_.copy_ops++;
    metrics_.bytes_processed += len;
    metrics_.last_checksum = final_hash;

    return final_hash;
}

// -----------------------------------------------------------------------------
// Path 2: Zero-Copy In-Place Path
// (Direct linear memory transformation without host-guest memcpy or allocator calls)
// -----------------------------------------------------------------------------
uint64_t VXRKernel::execute_zero_copy_path(size_t offset, size_t len) {
    if (!initialized_ || len == 0) {
        return 0;
    }

    uint8_t* raw_storage = ring_buffer_.get_raw_storage();
    if (!raw_storage || (offset + len > ring_buffer_.get_capacity())) {
        return 0;
    }

    uint8_t* target_ptr = raw_storage + offset;

    // Direct in-place compute workload (zero allocations, zero host-guest boundary copies)
    uint64_t initial_hash = compute_checksum(target_ptr, len);
    transform_payload(target_ptr, len, initial_hash);
    uint64_t final_hash = compute_checksum(target_ptr, len);

    metrics_.total_ops++;
    metrics_.zero_copy_ops++;
    metrics_.bytes_processed += len;
    metrics_.last_checksum = final_hash;

    return final_hash;
}

VXRKernel& get_global_kernel() {
    static VXRKernel s_instance;
    return s_instance;
}

} // namespace vxr
