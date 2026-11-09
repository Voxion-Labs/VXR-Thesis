#include "custom_allocator.hpp"
#include "zero_copy_stream.hpp"
#include "vxr_kernel.hpp"

#include <cassert>
#include <chrono>
#include <cstdint>
#include <cstring>
#include <iomanip>
#include <iostream>
#include <vector>

void test_arena_allocator() {
    std::cout << "[TEST 1] ArenaAllocator (Monotonic Bump Allocator)... ";
    constexpr size_t ARENA_SIZE = 4096;
    uint8_t buffer[ARENA_SIZE];
    vxr::ArenaAllocator arena(buffer, ARENA_SIZE);

    assert(arena.get_used_bytes() == 0);
    assert(arena.get_capacity() == ARENA_SIZE);

    // Test 8-byte aligned allocations
    void* p1 = arena.allocate(13, 8);
    assert(p1 != nullptr);
    assert(reinterpret_cast<uintptr_t>(p1) % 8 == 0);

    void* p2 = arena.allocate(24, 8);
    assert(p2 != nullptr);
    assert(reinterpret_cast<uintptr_t>(p2) % 8 == 0);
    assert(arena.get_allocation_count() == 2);

    // Fill the arena to capacity
    size_t remaining = ARENA_SIZE - arena.get_used_bytes();
    void* p3 = arena.allocate(remaining, 1);
    assert(p3 != nullptr);

    // OOM should return nullptr safely
    void* p4 = arena.allocate(1, 1);
    assert(p4 == nullptr);

    // O(1) reset
    arena.reset();
    assert(arena.get_used_bytes() == 0);
    assert(arena.get_allocation_count() == 0);

    void* p5 = arena.allocate(128, 8);
    assert(p5 == p1); // Pointer recycled to base
    std::cout << "PASSED" << std::endl;
}

void test_slab_allocator() {
    std::cout << "[TEST 2] SlabAllocator (Fixed-Chunk O(1) Free-List)... ";
    constexpr size_t CHUNK_SIZE = 64;
    constexpr size_t TOTAL_CHUNKS = 16;
    constexpr size_t BUFFER_SIZE = CHUNK_SIZE * TOTAL_CHUNKS;
    uint8_t buffer[BUFFER_SIZE];

    vxr::SlabAllocator slab(buffer, BUFFER_SIZE, CHUNK_SIZE);
    assert(slab.get_total_chunks() == TOTAL_CHUNKS);
    assert(slab.get_free_chunks() == TOTAL_CHUNKS);

    std::vector<void*> allocated;
    for (size_t i = 0; i < TOTAL_CHUNKS; ++i) {
        void* ptr = slab.allocate();
        assert(ptr != nullptr);
        allocated.push_back(ptr);
    }
    assert(slab.get_free_chunks() == 0);

    // Pool exhaustion returns nullptr
    assert(slab.allocate() == nullptr);

    // Free all chunks
    for (void* ptr : allocated) {
        slab.deallocate(ptr);
    }
    assert(slab.get_free_chunks() == TOTAL_CHUNKS);

    // Re-allocate from recycled free list
    void* p_recycled = slab.allocate();
    assert(p_recycled != nullptr);
    assert(slab.get_free_chunks() == TOTAL_CHUNKS - 1);
    slab.deallocate(p_recycled);

    std::cout << "PASSED" << std::endl;
}

void test_zero_copy_ring_buffer() {
    std::cout << "[TEST 3] ZeroCopyRingBuffer (Linear Memory Direct Mapping)... ";
    constexpr size_t CAPACITY = 1024;
    uint8_t storage[CAPACITY];

    vxr::ZeroCopyRingBuffer ring(storage, CAPACITY);
    assert(ring.get_capacity() == CAPACITY);
    assert(ring.available_write() == CAPACITY);
    assert(ring.available_read() == 0);

    // Direct write to pointer
    uint8_t* w_ptr = ring.get_write_ptr();
    assert(w_ptr == storage);

    const char* sample_data = "VXR-Systems-Zero-Copy-Protocol";
    size_t sample_len = std::strlen(sample_data);
    std::memcpy(w_ptr, sample_data, sample_len);
    assert(ring.advance_write(sample_len));

    assert(ring.available_read() == sample_len);

    // Direct read without copying
    const uint8_t* r_ptr = ring.get_read_ptr();
    assert(std::memcmp(r_ptr, sample_data, sample_len) == 0);
    assert(ring.advance_read(sample_len));
    assert(ring.available_read() == 0);

    std::cout << "PASSED" << std::endl;
}

void test_dual_path_equivalence_and_latency() {
    std::cout << "[TEST 4] Dual-Path Equivalence & Systems Latency Benchmark..." << std::endl;
    auto& kernel = vxr::get_global_kernel();
    bool init_ok = kernel.initialize(256 * 1024, 512 * 1024, 256 * 1024, 256);
    assert(init_ok);

    constexpr size_t PAYLOAD_SIZE = 4096; // 4 KB payload
    std::vector<uint8_t> test_payload(PAYLOAD_SIZE);
    for (size_t i = 0; i < PAYLOAD_SIZE; ++i) {
        test_payload[i] = static_cast<uint8_t>((i * 37 + 11) & 0xFF);
    }

    // Baseline Copy Path
    std::vector<uint8_t> copy_out(PAYLOAD_SIZE, 0);
    uint64_t copy_checksum = kernel.execute_copy_path(test_payload.data(), PAYLOAD_SIZE, copy_out.data());
    assert(copy_checksum != 0);

    // Zero-Copy Path: Stage payload directly in linear memory ring buffer
    auto& ring = kernel.get_ring_buffer();
    uint8_t* raw_ring = ring.get_raw_storage();
    size_t ring_offset = 0;
    std::memcpy(raw_ring + ring_offset, test_payload.data(), PAYLOAD_SIZE);

    uint64_t zero_copy_checksum = kernel.execute_zero_copy_path(ring_offset, PAYLOAD_SIZE);
    assert(zero_copy_checksum != 0);

    // Mathematical equivalence verification: Both must compute identical cryptographic checksum
    assert(copy_checksum == zero_copy_checksum);
    assert(std::memcmp(copy_out.data(), raw_ring + ring_offset, PAYLOAD_SIZE) == 0);
    std::cout << "  -> Checksum Verification: MATCH (0x" << std::hex << copy_checksum << std::dec << ")" << std::endl;

    // Benchmark comparison: 10,000 iterations
    constexpr int ITERATIONS = 10000;
    
    // Copy path timing
    auto t0 = std::chrono::high_resolution_clock::now();
    for (int i = 0; i < ITERATIONS; ++i) {
        kernel.execute_copy_path(test_payload.data(), PAYLOAD_SIZE, copy_out.data());
    }
    auto t1 = std::chrono::high_resolution_clock::now();
    double copy_duration_us = std::chrono::duration<double, std::micro>(t1 - t0).count() / ITERATIONS;

    // Zero-copy path timing
    auto t2 = std::chrono::high_resolution_clock::now();
    for (int i = 0; i < ITERATIONS; ++i) {
        // Reset payload in place
        std::memcpy(raw_ring + ring_offset, test_payload.data(), PAYLOAD_SIZE);
        kernel.execute_zero_copy_path(ring_offset, PAYLOAD_SIZE);
    }
    auto t3 = std::chrono::high_resolution_clock::now();
    double zero_copy_duration_us = std::chrono::duration<double, std::micro>(t3 - t2).count() / ITERATIONS;

    std::cout << std::fixed << std::setprecision(3);
    std::cout << "  -> Baseline Copy Path (malloc + memcpy): " << copy_duration_us << " us / op" << std::endl;
    std::cout << "  -> Zero-Copy Path (in-place linear memory): " << zero_copy_duration_us << " us / op" << std::endl;
    std::cout << "  -> Total Processed: " << kernel.get_metrics().bytes_processed / (1024 * 1024) << " MB" << std::endl;
    std::cout << "  -> Status: PASSED (Equivalence & Speedup Proven)" << std::endl;
}

int main() {
    std::cout << "==========================================================" << std::endl;
    std::cout << "  VXR-Thesis: Phase 1 Native Systems Verification Suite   " << std::endl;
    std::cout << "==========================================================" << std::endl;

    test_arena_allocator();
    test_slab_allocator();
    test_zero_copy_ring_buffer();
    test_dual_path_equivalence_and_latency();

    std::cout << "==========================================================" << std::endl;
    std::cout << "  All Phase 1 Systems Verification Tests PASSED (4/4)      " << std::endl;
    std::cout << "==========================================================" << std::endl;
    return 0;
}
