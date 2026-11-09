#include "vxr_kernel.hpp"
#include <cstdint>

#ifdef __EMSCRIPTEN__
#include <emscripten.h>
#define VXR_EXPORT EMSCRIPTEN_KEEPALIVE
#else
#define VXR_EXPORT
#endif

extern "C" {

VXR_EXPORT int32_t vxr_init(uint32_t ring_buffer_size,
                            uint32_t arena_size,
                            uint32_t slab_size,
                            uint32_t slab_chunk_size) {
    auto& kernel = vxr::get_global_kernel();
    bool ok = kernel.initialize(
        ring_buffer_size == 0 ? 256 * 1024 : ring_buffer_size,
        arena_size == 0 ? 512 * 1024 : arena_size,
        slab_size == 0 ? 256 * 1024 : slab_size,
        slab_chunk_size == 0 ? 256 : slab_chunk_size
    );
    return ok ? 1 : 0;
}

VXR_EXPORT void* vxr_get_ring_buffer_ptr() {
    auto& kernel = vxr::get_global_kernel();
    return kernel.get_ring_buffer().get_raw_storage();
}

VXR_EXPORT uint32_t vxr_get_ring_buffer_capacity() {
    auto& kernel = vxr::get_global_kernel();
    return static_cast<uint32_t>(kernel.get_ring_buffer().get_capacity());
}

VXR_EXPORT void* vxr_alloc_arena(uint32_t size) {
    auto& kernel = vxr::get_global_kernel();
    return kernel.get_arena().allocate(size);
}

VXR_EXPORT void vxr_reset_arena() {
    auto& kernel = vxr::get_global_kernel();
    kernel.get_arena().reset();
}

VXR_EXPORT void* vxr_alloc_slab() {
    auto& kernel = vxr::get_global_kernel();
    return kernel.get_slab().allocate();
}

VXR_EXPORT void vxr_free_slab(void* ptr) {
    auto& kernel = vxr::get_global_kernel();
    kernel.get_slab().deallocate(ptr);
}

VXR_EXPORT uint64_t vxr_process_copy(const uint8_t* in_ptr, uint32_t in_len, uint8_t* out_ptr) {
    auto& kernel = vxr::get_global_kernel();
    return kernel.execute_copy_path(in_ptr, in_len, out_ptr);
}

VXR_EXPORT uint64_t vxr_process_zero_copy(uint32_t offset, uint32_t len) {
    auto& kernel = vxr::get_global_kernel();
    return kernel.execute_zero_copy_path(offset, len);
}

VXR_EXPORT uint64_t vxr_compute_checksum(const uint8_t* ptr, uint32_t len) {
    return vxr::VXRKernel::compute_checksum(ptr, len);
}

VXR_EXPORT uint64_t vxr_get_stats_total_ops() {
    auto& kernel = vxr::get_global_kernel();
    return kernel.get_metrics().total_ops;
}

VXR_EXPORT uint64_t vxr_get_stats_copy_ops() {
    auto& kernel = vxr::get_global_kernel();
    return kernel.get_metrics().copy_ops;
}

VXR_EXPORT uint64_t vxr_get_stats_zero_copy_ops() {
    auto& kernel = vxr::get_global_kernel();
    return kernel.get_metrics().zero_copy_ops;
}

VXR_EXPORT uint64_t vxr_get_stats_bytes_processed() {
    auto& kernel = vxr::get_global_kernel();
    return kernel.get_metrics().bytes_processed;
}

VXR_EXPORT uint64_t vxr_get_stats_last_checksum() {
    auto& kernel = vxr::get_global_kernel();
    return kernel.get_metrics().last_checksum;
}

} // extern "C"
