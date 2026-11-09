#include "custom_allocator.hpp"
#include <cstring>

namespace vxr {

// -----------------------------------------------------------------------------
// ArenaAllocator Implementation
// -----------------------------------------------------------------------------

ArenaAllocator::ArenaAllocator(uint8_t* buffer, size_t capacity) {
    init(buffer, capacity);
}

void ArenaAllocator::init(uint8_t* buffer, size_t capacity) {
    buffer_ = buffer;
    capacity_ = capacity;
    offset_ = 0;
    alloc_count_ = 0;
}

void* ArenaAllocator::allocate(size_t bytes, size_t alignment) {
    if (!buffer_ || bytes == 0) {
        return nullptr;
    }

    size_t aligned_offset = align_up(offset_, alignment);
    if (aligned_offset + bytes > capacity_) {
        return nullptr; // Out of memory
    }

    void* ptr = buffer_ + aligned_offset;
    offset_ = aligned_offset + bytes;
    alloc_count_++;
    return ptr;
}

void ArenaAllocator::reset() {
    offset_ = 0;
    alloc_count_ = 0;
}

// -----------------------------------------------------------------------------
// SlabAllocator Implementation
// -----------------------------------------------------------------------------

SlabAllocator::SlabAllocator(uint8_t* buffer, size_t total_size, size_t chunk_size) {
    init(buffer, total_size, chunk_size);
}

void SlabAllocator::init(uint8_t* buffer, size_t total_size, size_t chunk_size) {
    buffer_ = buffer;
    total_size_ = total_size;

    // Minimum chunk size must fit a FreeNode pointer and be 8-byte aligned
    size_t min_size = sizeof(FreeNode);
    chunk_size_ = align_up(chunk_size < min_size ? min_size : chunk_size, 8);

    total_chunks_ = total_size_ / chunk_size_;
    free_chunks_ = total_chunks_;
    free_list_ = nullptr;

    // Wire up the embedded free-list
    for (size_t i = 0; i < total_chunks_; ++i) {
        auto* node = reinterpret_cast<FreeNode*>(buffer_ + (i * chunk_size_));
        node->next = free_list_;
        free_list_ = node;
    }
}

void* SlabAllocator::allocate() {
    if (!free_list_) {
        return nullptr; // Slab pool exhausted
    }

    FreeNode* node = free_list_;
    free_list_ = node->next;
    free_chunks_--;
    return reinterpret_cast<void*>(node);
}

void SlabAllocator::deallocate(void* ptr) {
    if (!ptr || !buffer_) {
        return;
    }

    auto* node = reinterpret_cast<FreeNode*>(ptr);
    node->next = free_list_;
    free_list_ = node;
    free_chunks_++;
}

} // namespace vxr
