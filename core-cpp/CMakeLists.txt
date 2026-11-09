cmake_minimum_required(VERSION 3.14)
project(vxr_kernel CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_EXPORT_COMPILE_COMMANDS ON)

# Optimization flags for ultra-low latency execution
if(MSVC)
    add_compile_options(/O2 /W4)
else()
    add_compile_options(-O3 -Wall -Wextra -Wpedantic)
endif()

include_directories(include)

set(VXR_SOURCES
    src/custom_allocator.cpp
    src/zero_copy_stream.cpp
    src/vxr_kernel.cpp
    src/bindings.cpp
)

if(EMSCRIPTEN)
    # Standalone WebAssembly kernel output
    add_executable(vxr_kernel ${VXR_SOURCES})
    set_target_properties(vxr_kernel PROPERTIES
        OUTPUT_NAME "vxr_kernel"
        SUFFIX ".wasm"
        LINK_FLAGS "-O3 -flto -s STANDALONE_WASM=1 -s EXPORTED_FUNCTIONS=\"['_vxr_init', '_vxr_get_ring_buffer_ptr', '_vxr_get_ring_buffer_capacity', '_vxr_alloc_arena', '_vxr_reset_arena', '_vxr_alloc_slab', '_vxr_free_slab', '_vxr_process_copy', '_vxr_process_zero_copy', '_vxr_compute_checksum', '_vxr_get_stats_total_ops', '_vxr_get_stats_copy_ops', '_vxr_get_stats_zero_copy_ops', '_vxr_get_stats_bytes_processed', '_vxr_get_stats_last_checksum']\" -s ERROR_ON_UNDEFINED_SYMBOLS=0 --no-entry"
    )
else()
    # Native static library for native benchmark & verification
    add_library(vxr_kernel_static STATIC ${VXR_SOURCES})

    # Native test runner executable
    if(EXISTS "${CMAKE_CURRENT_SOURCE_DIR}/tests/test_kernel.cpp")
        add_executable(test_kernel tests/test_kernel.cpp)
        target_link_libraries(test_kernel PRIVATE vxr_kernel_static)
    endif()
endif()
