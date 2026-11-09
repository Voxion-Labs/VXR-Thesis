#pragma once

#include <cstddef>
#include <cstdint>

namespace vxr {

// Align up to power of two
inline constexpr size_t align_up(size_t size, size_t alignment) {
    return (size + (alignment - 1)) & ~(alignment - 1);
}

/**
 * @brief ArenaAllocator (Monotonic Bump Allocator)
 * 
 * Provides O(1) contiguous allocation with zero runtime header overhead.
 * All memory is freed at once in O(1) via reset().
 */
class ArenaAllocator {
public:
    ArenaAllocator() = default;
    ArenaAllocator(uint8_t* buffer, size_t capacity);

    void init(uint8_t* buffer, size_t capacity);

    // Allocate aligned bytes. Returns nullptr if capacity exceeded.
    void* allocate(size_t bytes, size_t alignment = 8);

    // O(1) reset of the entire arena pointer
    void reset();

    // Query metrics
    size_t get_used_bytes() const { return offset_; }
    size_t get_capacity() const { return capacity_; }
    size_t get_allocation_count() const { return alloc_count_; }

private:
    uint8_t* buffer_ = nullptr;
    size_t capacity_ = 0;
    size_t offset_ = 0;
    size_t alloc_count_ = 0;
};

/**
 * @brief SlabAllocator (Fixed-Chunk Memory Pool)
 * 
 * Eliminates external fragmentation by maintaining an embedded free-list
 * of fixed-size chunks. Guarantees strictly O(1) allocate and O(1) deallocate.
 */
class SlabAllocator {
public:
    SlabAllocator() = default;
    SlabAllocator(uint8_t* buffer, size_t total_size, size_t chunk_size);

    void init(uint8_t* buffer, size_t total_size, size_t chunk_size);

    // O(1) pop from free list
    void* allocate();

    // O(1) push to free list
    void deallocate(void* ptr);

    size_t get_free_chunks() const { return free_chunks_; }
    size_t get_total_chunks() const { return total_chunks_; }
    size_t get_chunk_size() const { return chunk_size_; }

private:
    struct FreeNode {
        FreeNode* next;
    };

    uint8_t* buffer_ = nullptr;
    size_t total_size_ = 0;
    size_t chunk_size_ = 0;
    size_t total_chunks_ = 0;
    size_t free_chunks_ = 0;
    FreeNode* free_list_ = nullptr;
};

} // namespace vxr
