#include "zero_copy_stream.hpp"
#include <cstring>

namespace vxr {

ZeroCopyRingBuffer::ZeroCopyRingBuffer(uint8_t* storage, size_t capacity) {
    init(storage, capacity);
}

void ZeroCopyRingBuffer::init(uint8_t* storage, size_t capacity) {
    storage_ = storage;
    
    // Ensure capacity is a power of 2 for fast bitwise masking
    size_t pow2 = 1;
    while (pow2 < capacity) {
        pow2 <<= 1;
    }
    capacity_ = pow2;
    mask_ = capacity_ - 1;
    head_ = 0;
    tail_ = 0;
}

uint8_t* ZeroCopyRingBuffer::get_write_ptr() {
    if (!storage_) return nullptr;
    return storage_ + (head_ & mask_);
}

const uint8_t* ZeroCopyRingBuffer::get_read_ptr() const {
    if (!storage_) return nullptr;
    return storage_ + (tail_ & mask_);
}

bool ZeroCopyRingBuffer::advance_write(size_t bytes) {
    if (available_write() < bytes) {
        return false;
    }
    head_ += bytes;
    return true;
}

bool ZeroCopyRingBuffer::advance_read(size_t bytes) {
    if (available_read() < bytes) {
        return false;
    }
    tail_ += bytes;
    return true;
}

size_t ZeroCopyRingBuffer::available_write() const {
    return capacity_ - (head_ - tail_);
}

size_t ZeroCopyRingBuffer::available_read() const {
    return head_ - tail_;
}

void ZeroCopyRingBuffer::reset() {
    head_ = 0;
    tail_ = 0;
}

} // namespace vxr
