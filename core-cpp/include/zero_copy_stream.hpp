#pragma once

#include <cstddef>
#include <cstdint>

namespace vxr {

#pragma pack(push, 1)
// Telemetry packet header embedded directly in zero-copy stream
struct StreamPacketHeader {
    uint32_t magic;         // 0x56585231 ("VXR1")
    uint32_t payload_len;   // Length of payload following header
    uint64_t sequence_id;   // Monotonic sequence number
    uint64_t timestamp_ns;  // Host micro/nanosecond timestamp
    uint64_t checksum;      // Payload validation hash
};
#pragma pack(pop)

/**
 * @brief ZeroCopyRingBuffer
 * 
 * High-performance circular buffer residing inside Wasm linear memory.
 * Bypasses host-guest boundary copies: Host writes directly to write_ptr,
 * Wasm kernel executes in-place, and Host reads back results at the same offset.
 */
class ZeroCopyRingBuffer {
public:
    ZeroCopyRingBuffer() = default;
    ZeroCopyRingBuffer(uint8_t* storage, size_t capacity);

    void init(uint8_t* storage, size_t capacity);

    // Host interface: get raw memory address where host should write
    uint8_t* get_raw_storage() const { return storage_; }
    size_t get_capacity() const { return capacity_; }

    // Direct pointer to current write position in linear memory
    uint8_t* get_write_ptr();

    // Direct pointer to current read position in linear memory
    const uint8_t* get_read_ptr() const;

    // Advance write offset after host has directly written bytes
    bool advance_write(size_t bytes);

    // Advance read offset after kernel has processed bytes
    bool advance_read(size_t bytes);

    // Available space for writing
    size_t available_write() const;

    // Available bytes waiting to be processed
    size_t available_read() const;

    // Reset offsets
    void reset();

private:
    uint8_t* storage_ = nullptr;
    size_t capacity_ = 0;
    size_t mask_ = 0;
    size_t head_ = 0; // Write position
    size_t tail_ = 0; // Read position
};

} // namespace vxr
