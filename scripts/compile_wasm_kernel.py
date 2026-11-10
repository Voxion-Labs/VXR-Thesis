#!/usr/bin/env python3
"""
VXR-Thesis Standalone WebAssembly Kernel Synthesizer
Generates a valid, spec-compliant standalone WebAssembly module (vxr_kernel.wasm)
implementing the VXR low-level systems ABI:
- 64-bit FNV-1a non-cryptographic checksum
- Streaming in-place payload transformation
- Direct linear memory zero-copy ring buffer (offset 0x10000 = 64KB)
- Monotonic arena allocator (offset 0x50000 = 320KB)
- O(1) Slab chunk allocator
- Full telemetry counters
Zero host software dependencies required (standard library Python 3).
"""

import os
import struct
import sys

# -----------------------------------------------------------------------------
# LEB128 Encoding Utilities
# -----------------------------------------------------------------------------

def encode_u32(val: int) -> bytes:
    res = bytearray()
    while True:
        b = val & 0x7F
        val >>= 7
        if val != 0:
            res.append(b | 0x80)
        else:
            res.append(b)
            break
    return bytes(res)


def encode_s32(val: int) -> bytes:
    res = bytearray()
    more = True
    while more:
        b = val & 0x7F
        val >>= 7
        if (val == 0 and not (b & 0x40)) or (val == -1 and (b & 0x40)):
            more = False
        else:
            b |= 0x80
        res.append(b)
    return bytes(res)


def encode_s64(val: int) -> bytes:
    res = bytearray()
    more = True
    while more:
        b = val & 0x7F
        val >>= 7
        if (val == 0 and not (b & 0x40)) or (val == -1 and (b & 0x40)):
            more = False
        else:
            b |= 0x80
        res.append(b)
    return bytes(res)


def make_section(sec_id: int, payload: bytes) -> bytes:
    return bytes([sec_id]) + encode_u32(len(payload)) + payload


def make_vector(items: list) -> bytes:
    return encode_u32(len(items)) + b"".join(items)


# -----------------------------------------------------------------------------
# WebAssembly Value Types & Opcodes
# -----------------------------------------------------------------------------

TYPE_I32 = 0x7F
TYPE_I64 = 0x7E
TYPE_EMPTY = 0x40

OP_UNREACHABLE = 0x00
OP_BLOCK       = 0x02
OP_LOOP        = 0x03
OP_IF          = 0x04
OP_ELSE        = 0x05
OP_END         = 0x0B
OP_BR          = 0x0C
OP_BR_IF       = 0x0D
OP_RETURN      = 0x0F
OP_DROP        = 0x1A

OP_LOCAL_GET   = 0x20
OP_LOCAL_SET   = 0x21
OP_LOCAL_TEE   = 0x22
OP_GLOBAL_GET  = 0x23
OP_GLOBAL_SET  = 0x24

OP_I32_LOAD    = 0x28
OP_I32_LOAD8_U = 0x2D
OP_I32_STORE   = 0x36
OP_I32_STORE8  = 0x3A

OP_I32_CONST   = 0x41
OP_I64_CONST   = 0x42

OP_I32_EQ      = 0x46
OP_I32_NE      = 0x47
OP_I32_LT_U    = 0x49
OP_I32_GT_U    = 0x4B
OP_I32_GE_U    = 0x4E

OP_I32_ADD     = 0x6A
OP_I32_SUB     = 0x6B
OP_I32_MUL     = 0x6C
OP_I32_AND     = 0x71
OP_I32_XOR     = 0x73

OP_I64_ADD     = 0x7C
OP_I64_MUL     = 0x7E
OP_I64_AND     = 0x83
OP_I64_XOR     = 0x85

OP_I32_WRAP_I64     = 0xA7
OP_I64_EXTEND_I32_U = 0xAC

# Memory Layout Offsets in Guest Linear Memory
# Page 0 (0 - 64KB): Stack & Scratch
# Page 1-4 (64KB - 320KB): Zero-Copy Ring Buffer (256 KB)
# Page 5-12 (320KB - 832KB): Arena Allocator (512 KB)
# Page 13-16 (832KB - 1088KB): Slab Allocator (256 KB)
RING_BUFFER_BASE     = 65536         # 0x10000 (64 KB)
RING_BUFFER_CAPACITY = 256 * 1024    # 262,144 bytes
ARENA_BASE           = 327680        # 0x50000 (320 KB)
ARENA_CAPACITY       = 512 * 1024    # 524,288 bytes

FNV_PRIME = 1099511628211
# Unsigned 64-bit 14695981039346656037 as signed 64-bit:
FNV_OFFSET_BASIS = -3750763034362895579


def build_wasm_binary() -> bytes:
    # -------------------------------------------------------------------------
    # 1. Type Section
    # -------------------------------------------------------------------------
    # Signature 0: () -> i32
    sig_0 = bytes([0x60, 0x00, 0x01, TYPE_I32])
    # Signature 1: () -> void
    sig_1 = bytes([0x60, 0x00, 0x00])
    # Signature 2: (i32) -> i32
    sig_2 = bytes([0x60, 0x01, TYPE_I32, 0x01, TYPE_I32])
    # Signature 3: (i32) -> void
    sig_3 = bytes([0x60, 0x01, TYPE_I32, 0x00])
    # Signature 4: (i32, i32) -> i64
    sig_4 = bytes([0x60, 0x02, TYPE_I32, TYPE_I32, 0x01, TYPE_I64])
    # Signature 5: (i32, i32, i32) -> i64
    sig_5 = bytes([0x60, 0x03, TYPE_I32, TYPE_I32, TYPE_I32, 0x01, TYPE_I64])
    # Signature 6: (i32, i32, i32, i32) -> i32 (for vxr_init)
    sig_6 = bytes([0x60, 0x04, TYPE_I32, TYPE_I32, TYPE_I32, TYPE_I32, 0x01, TYPE_I32])
    # Signature 7: () -> i64
    sig_7 = bytes([0x60, 0x00, 0x01, TYPE_I64])

    type_section = make_section(1, make_vector([
        sig_0, sig_1, sig_2, sig_3, sig_4, sig_5, sig_6, sig_7
    ]))

    # -------------------------------------------------------------------------
    # 2. Function Section: maps function index to type signature index
    # -------------------------------------------------------------------------
    # fn 0:  vxr_init                   -> sig 6 (i32, i32, i32, i32) -> i32
    # fn 1:  vxr_get_ring_buffer_ptr    -> sig 0 () -> i32
    # fn 2:  vxr_get_ring_buffer_capacity -> sig 0 () -> i32
    # fn 3:  vxr_alloc_arena            -> sig 2 (i32) -> i32
    # fn 4:  vxr_reset_arena            -> sig 1 () -> void
    # fn 5:  vxr_alloc_slab             -> sig 0 () -> i32
    # fn 6:  vxr_free_slab              -> sig 3 (i32) -> void
    # fn 7:  vxr_process_copy           -> sig 5 (i32, i32, i32) -> i64
    # fn 8:  vxr_process_zero_copy      -> sig 4 (i32, i32) -> i64
    # fn 9:  vxr_compute_checksum       -> sig 4 (i32, i32) -> i64
    # fn 10: vxr_get_stats_total_ops    -> sig 7 () -> i64
    # fn 11: vxr_get_stats_copy_ops     -> sig 7 () -> i64
    # fn 12: vxr_get_stats_zero_copy_ops-> sig 7 () -> i64
    # fn 13: vxr_get_stats_bytes_processed -> sig 7 () -> i64
    # fn 14: vxr_get_stats_last_checksum-> sig 7 () -> i64
    fn_types = [6, 0, 0, 2, 1, 0, 3, 5, 4, 4, 7, 7, 7, 7, 7]
    func_section = make_section(3, make_vector([encode_u32(t) for t in fn_types]))

    # -------------------------------------------------------------------------
    # 3. Memory Section
    # -------------------------------------------------------------------------
    # 1 memory, limits: has_max=1 (0x01), min=32 pages (2MB), max=64 pages (4MB)
    mem_entry = bytes([0x01]) + encode_u32(32) + encode_u32(64)
    memory_section = make_section(5, make_vector([mem_entry]))

    # -------------------------------------------------------------------------
    # 4. Global Section
    # -------------------------------------------------------------------------
    # Global 0 (i32 mut): arena_offset (initialized to ARENA_BASE)
    # Global 1 (i32 mut): slab_index (initialized to 0)
    # Global 2 (i64 mut): total_ops (0)
    # Global 3 (i64 mut): copy_ops (0)
    # Global 4 (i64 mut): zero_copy_ops (0)
    # Global 5 (i64 mut): bytes_processed (0)
    # Global 6 (i64 mut): last_checksum (0)
    def make_global(val_type, is_mut, init_op, init_val_encoded):
        return bytes([val_type, 0x01 if is_mut else 0x00, init_op]) + init_val_encoded + bytes([OP_END])

    globals_list = [
        make_global(TYPE_I32, True, OP_I32_CONST, encode_s32(ARENA_BASE)),
        make_global(TYPE_I32, True, OP_I32_CONST, encode_s32(0)),
        make_global(TYPE_I64, True, OP_I64_CONST, encode_s64(0)),
        make_global(TYPE_I64, True, OP_I64_CONST, encode_s64(0)),
        make_global(TYPE_I64, True, OP_I64_CONST, encode_s64(0)),
        make_global(TYPE_I64, True, OP_I64_CONST, encode_s64(0)),
        make_global(TYPE_I64, True, OP_I64_CONST, encode_s64(0)),
    ]
    global_section = make_section(6, make_vector(globals_list))

    # -------------------------------------------------------------------------
    # 5. Export Section
    # -------------------------------------------------------------------------
    exports = [
        ("memory", 0x02, 0),
        ("vxr_init", 0x00, 0),
        ("vxr_get_ring_buffer_ptr", 0x00, 1),
        ("vxr_get_ring_buffer_capacity", 0x00, 2),
        ("vxr_alloc_arena", 0x00, 3),
        ("vxr_reset_arena", 0x00, 4),
        ("vxr_alloc_slab", 0x00, 5),
        ("vxr_free_slab", 0x00, 6),
        ("vxr_process_copy", 0x00, 7),
        ("vxr_process_zero_copy", 0x00, 8),
        ("vxr_compute_checksum", 0x00, 9),
        ("vxr_get_stats_total_ops", 0x00, 10),
        ("vxr_get_stats_copy_ops", 0x00, 11),
        ("vxr_get_stats_zero_copy_ops", 0x00, 12),
        ("vxr_get_stats_bytes_processed", 0x00, 13),
        ("vxr_get_stats_last_checksum", 0x00, 14),
    ]
    export_entries = []
    for name, kind, idx in exports:
        name_bytes = name.encode("utf-8")
        entry = encode_u32(len(name_bytes)) + name_bytes + bytes([kind]) + encode_u32(idx)
        export_entries.append(entry)
    export_section = make_section(7, make_vector(export_entries))

    # -------------------------------------------------------------------------
    # 6. Code Section: Bytecode Implementations
    # -------------------------------------------------------------------------
    code_bodies = []

    # Fn 0: vxr_init(ring_size, arena_size, slab_size, slab_chunk_size) -> i32 (returns 1)
    body_0 = bytearray()
    body_0 += bytes([0x00]) # 0 locals
    body_0 += bytes([OP_I32_CONST]) + encode_s32(1)
    body_0 += bytes([OP_END])
    code_bodies.append(encode_u32(len(body_0)) + body_0)

    # Fn 1: vxr_get_ring_buffer_ptr() -> i32 (returns RING_BUFFER_BASE = 65536)
    body_1 = bytearray()
    body_1 += bytes([0x00])
    body_1 += bytes([OP_I32_CONST]) + encode_s32(RING_BUFFER_BASE)
    body_1 += bytes([OP_END])
    code_bodies.append(encode_u32(len(body_1)) + body_1)

    # Fn 2: vxr_get_ring_buffer_capacity() -> i32 (returns RING_BUFFER_CAPACITY = 262144)
    body_2 = bytearray()
    body_2 += bytes([0x00])
    body_2 += bytes([OP_I32_CONST]) + encode_s32(RING_BUFFER_CAPACITY)
    body_2 += bytes([OP_END])
    code_bodies.append(encode_u32(len(body_2)) + body_2)

    # Fn 3: vxr_alloc_arena(size: i32) -> i32
    # Local 0: size
    # Local 1: cur_offset (i32)
    # Local 2: aligned_offset (i32)
    body_3 = bytearray()
    body_3 += encode_u32(1) + encode_u32(2) + bytes([TYPE_I32]) # 2 locals of type i32
    # cur_offset = global.get 0
    body_3 += bytes([OP_GLOBAL_GET, 0x00, OP_LOCAL_SET, 0x01])
    # aligned_offset = (cur_offset + 7) & ~7
    body_3 += bytes([OP_LOCAL_GET, 0x01, OP_I32_CONST]) + encode_s32(7) + bytes([OP_I32_ADD])
    body_3 += bytes([OP_I32_CONST]) + encode_s32(-8) + bytes([OP_I32_AND, OP_LOCAL_SET, 0x02])
    # if (aligned_offset + size > ARENA_BASE + ARENA_CAPACITY) return 0
    body_3 += bytes([OP_LOCAL_GET, 0x02, OP_LOCAL_GET, 0x00, OP_I32_ADD])
    body_3 += bytes([OP_I32_CONST]) + encode_s32(ARENA_BASE + ARENA_CAPACITY)
    body_3 += bytes([OP_I32_GT_U])
    body_3 += bytes([OP_IF, TYPE_EMPTY, OP_I32_CONST, 0x00, OP_RETURN, OP_END])
    # global.set 0 = aligned_offset + size
    body_3 += bytes([OP_LOCAL_GET, 0x02, OP_LOCAL_GET, 0x00, OP_I32_ADD, OP_GLOBAL_SET, 0x00])
    # return aligned_offset
    body_3 += bytes([OP_LOCAL_GET, 0x02, OP_END])
    code_bodies.append(encode_u32(len(body_3)) + body_3)

    # Fn 4: vxr_reset_arena() -> void
    body_4 = bytearray()
    body_4 += bytes([0x00]) # 0 locals
    body_4 += bytes([OP_I32_CONST]) + encode_s32(ARENA_BASE) + bytes([OP_GLOBAL_SET, 0x00])
    body_4 += bytes([OP_END])
    code_bodies.append(encode_u32(len(body_4)) + body_4)

    # Fn 5: vxr_alloc_slab() -> i32
    # Returns chunk at offset 851968 + (slab_idx * 256)
    body_5 = bytearray()
    body_5 += bytes([0x00])
    body_5 += bytes([OP_GLOBAL_GET, 0x01, OP_I32_CONST]) + encode_s32(256) + bytes([OP_I32_MUL])
    body_5 += bytes([OP_I32_CONST]) + encode_s32(ARENA_BASE + ARENA_CAPACITY) + bytes([OP_I32_ADD])
    # slab_index = (slab_index + 1) & 0x3FF (1024 chunks)
    body_5 += bytes([OP_GLOBAL_GET, 0x01, OP_I32_CONST, 0x01, OP_I32_ADD])
    body_5 += bytes([OP_I32_CONST]) + encode_s32(0x3FF) + bytes([OP_I32_AND, OP_GLOBAL_SET, 0x01])
    body_5 += bytes([OP_END])
    code_bodies.append(encode_u32(len(body_5)) + body_5)

    # Fn 6: vxr_free_slab(ptr: i32) -> void (no-op in monotonic pool)
    body_6 = bytearray()
    body_6 += bytes([0x00])
    body_6 += bytes([OP_END])
    code_bodies.append(encode_u32(len(body_6)) + body_6)

    # Fn 7: vxr_process_copy(in_ptr: i32, in_len: i32, out_ptr: i32) -> i64
    # Param 0: in_ptr, Param 1: in_len, Param 2: out_ptr
    # Local 3: hash (i64), Local 4: i (i32), Local 5: mask (i32), Local 6: val (i32)
    body_7 = bytearray()
    body_7 += encode_u32(2)
    body_7 += encode_u32(1) + bytes([TYPE_I64]) # local 3: i64 hash
    body_7 += encode_u32(3) + bytes([TYPE_I32]) # local 4: i, local 5: mask, local 6: val
    # Step 1: compute initial hash
    body_7 += bytes([OP_I64_CONST]) + encode_s64(FNV_OFFSET_BASIS) + bytes([OP_LOCAL_SET, 0x03])
    body_7 += bytes([OP_I32_CONST, 0x00, OP_LOCAL_SET, 0x04])
    body_7 += bytes([OP_BLOCK, TYPE_EMPTY, OP_LOOP, TYPE_EMPTY])
    body_7 += bytes([OP_LOCAL_GET, 0x04, OP_LOCAL_GET, 0x01, OP_I32_GE_U, OP_BR_IF, 0x01])
    body_7 += bytes([OP_LOCAL_GET, 0x00, OP_LOCAL_GET, 0x04, OP_I32_ADD, OP_I32_LOAD8_U, 0x00, 0x00])
    body_7 += bytes([OP_I64_EXTEND_I32_U, OP_LOCAL_GET, 0x03, OP_I64_XOR])
    body_7 += bytes([OP_I64_CONST]) + encode_s64(FNV_PRIME) + bytes([OP_I64_MUL, OP_LOCAL_SET, 0x03])
    body_7 += bytes([OP_LOCAL_GET, 0x04, OP_I32_CONST, 0x01, OP_I32_ADD, OP_LOCAL_SET, 0x04])
    body_7 += bytes([OP_BR, 0x00, OP_END, OP_END])
    # mask = i32.wrap_i64(hash) & 0xFF
    body_7 += bytes([OP_LOCAL_GET, 0x03, OP_I32_WRAP_I64, OP_I32_CONST]) + encode_s32(0xFF) + bytes([OP_I32_AND, OP_LOCAL_SET, 0x05])

    # Step 2: transform while copying from in_ptr to out_ptr
    body_7 += bytes([OP_I32_CONST, 0x00, OP_LOCAL_SET, 0x04])
    body_7 += bytes([OP_BLOCK, TYPE_EMPTY, OP_LOOP, TYPE_EMPTY])
    body_7 += bytes([OP_LOCAL_GET, 0x04, OP_LOCAL_GET, 0x01, OP_I32_GE_U, OP_BR_IF, 0x01])
    # val = in[i] ^ (mask + (i & 0xFF))
    body_7 += bytes([OP_LOCAL_GET, 0x00, OP_LOCAL_GET, 0x04, OP_I32_ADD, OP_I32_LOAD8_U, 0x00, 0x00])
    body_7 += bytes([OP_LOCAL_GET, 0x05, OP_LOCAL_GET, 0x04, OP_I32_CONST]) + encode_s32(0xFF) + bytes([OP_I32_AND, OP_I32_ADD, OP_I32_XOR, OP_LOCAL_SET, 0x06])
    # out[i] = val
    body_7 += bytes([OP_LOCAL_GET, 0x02, OP_LOCAL_GET, 0x04, OP_I32_ADD, OP_LOCAL_GET, 0x06, OP_I32_STORE8, 0x00, 0x00])
    body_7 += bytes([OP_LOCAL_GET, 0x04, OP_I32_CONST, 0x01, OP_I32_ADD, OP_LOCAL_SET, 0x04])
    body_7 += bytes([OP_BR, 0x00, OP_END, OP_END])

    # Step 3: compute final checksum on out_ptr
    body_7 += bytes([OP_I64_CONST]) + encode_s64(FNV_OFFSET_BASIS) + bytes([OP_LOCAL_SET, 0x03])
    body_7 += bytes([OP_I32_CONST, 0x00, OP_LOCAL_SET, 0x04])
    body_7 += bytes([OP_BLOCK, TYPE_EMPTY, OP_LOOP, TYPE_EMPTY])
    body_7 += bytes([OP_LOCAL_GET, 0x04, OP_LOCAL_GET, 0x01, OP_I32_GE_U, OP_BR_IF, 0x01])
    body_7 += bytes([OP_LOCAL_GET, 0x02, OP_LOCAL_GET, 0x04, OP_I32_ADD, OP_I32_LOAD8_U, 0x00, 0x00])
    body_7 += bytes([OP_I64_EXTEND_I32_U, OP_LOCAL_GET, 0x03, OP_I64_XOR])
    body_7 += bytes([OP_I64_CONST]) + encode_s64(FNV_PRIME) + bytes([OP_I64_MUL, OP_LOCAL_SET, 0x03])
    body_7 += bytes([OP_LOCAL_GET, 0x04, OP_I32_CONST, 0x01, OP_I32_ADD, OP_LOCAL_SET, 0x04])
    body_7 += bytes([OP_BR, 0x00, OP_END, OP_END])

    # Update telemetry globals
    # total_ops += 1, copy_ops += 1, bytes_processed += in_len, last_checksum = hash
    body_7 += bytes([OP_GLOBAL_GET, 0x02, OP_I64_CONST, 0x01, OP_I64_ADD, OP_GLOBAL_SET, 0x02])
    body_7 += bytes([OP_GLOBAL_GET, 0x03, OP_I64_CONST, 0x01, OP_I64_ADD, OP_GLOBAL_SET, 0x03])
    body_7 += bytes([OP_GLOBAL_GET, 0x05, OP_LOCAL_GET, 0x01, OP_I64_EXTEND_I32_U, OP_I64_ADD, OP_GLOBAL_SET, 0x05])
    body_7 += bytes([OP_LOCAL_GET, 0x03, OP_GLOBAL_SET, 0x06])

    # return final hash
    body_7 += bytes([OP_LOCAL_GET, 0x03, OP_END])
    code_bodies.append(encode_u32(len(body_7)) + body_7)

    # Fn 8: vxr_process_zero_copy(offset: i32, len: i32) -> i64
    # Param 0: offset, Param 1: len
    # Local 2: hash (i64), Local 3: i (i32), Local 4: mask (i32), Local 5: ptr (i32)
    body_8 = bytearray()
    body_8 += encode_u32(2)
    body_8 += encode_u32(1) + bytes([TYPE_I64]) # local 2: i64 hash
    body_8 += encode_u32(3) + bytes([TYPE_I32]) # local 3: i, local 4: mask, local 5: ptr
    # ptr = RING_BUFFER_BASE + offset
    body_8 += bytes([OP_I32_CONST]) + encode_s32(RING_BUFFER_BASE)
    body_8 += bytes([OP_LOCAL_GET, 0x00, OP_I32_ADD, OP_LOCAL_SET, 0x05])

    # Step 1: compute initial checksum
    body_8 += bytes([OP_I64_CONST]) + encode_s64(FNV_OFFSET_BASIS) + bytes([OP_LOCAL_SET, 0x02])
    body_8 += bytes([OP_I32_CONST, 0x00, OP_LOCAL_SET, 0x03])
    body_8 += bytes([OP_BLOCK, TYPE_EMPTY, OP_LOOP, TYPE_EMPTY])
    body_8 += bytes([OP_LOCAL_GET, 0x03, OP_LOCAL_GET, 0x01, OP_I32_GE_U, OP_BR_IF, 0x01])
    body_8 += bytes([OP_LOCAL_GET, 0x05, OP_LOCAL_GET, 0x03, OP_I32_ADD, OP_I32_LOAD8_U, 0x00, 0x00])
    body_8 += bytes([OP_I64_EXTEND_I32_U, OP_LOCAL_GET, 0x02, OP_I64_XOR])
    body_8 += bytes([OP_I64_CONST]) + encode_s64(FNV_PRIME) + bytes([OP_I64_MUL, OP_LOCAL_SET, 0x02])
    body_8 += bytes([OP_LOCAL_GET, 0x03, OP_I32_CONST, 0x01, OP_I32_ADD, OP_LOCAL_SET, 0x03])
    body_8 += bytes([OP_BR, 0x00, OP_END, OP_END])
    # mask = i32.wrap_i64(hash) & 0xFF
    body_8 += bytes([OP_LOCAL_GET, 0x02, OP_I32_WRAP_I64, OP_I32_CONST]) + encode_s32(0xFF) + bytes([OP_I32_AND, OP_LOCAL_SET, 0x04])

    # Step 2: in-place transform directly inside linear memory ring buffer
    body_8 += bytes([OP_I32_CONST, 0x00, OP_LOCAL_SET, 0x03])
    body_8 += bytes([OP_BLOCK, TYPE_EMPTY, OP_LOOP, TYPE_EMPTY])
    body_8 += bytes([OP_LOCAL_GET, 0x03, OP_LOCAL_GET, 0x01, OP_I32_GE_U, OP_BR_IF, 0x01])
    # ptr[i] = ptr[i] ^ (mask + (i & 0xFF))
    body_8 += bytes([OP_LOCAL_GET, 0x05, OP_LOCAL_GET, 0x03, OP_I32_ADD]) # destination address
    body_8 += bytes([OP_LOCAL_GET, 0x05, OP_LOCAL_GET, 0x03, OP_I32_ADD, OP_I32_LOAD8_U, 0x00, 0x00]) # val
    body_8 += bytes([OP_LOCAL_GET, 0x04, OP_LOCAL_GET, 0x03, OP_I32_CONST]) + encode_s32(0xFF) + bytes([OP_I32_AND, OP_I32_ADD, OP_I32_XOR])
    body_8 += bytes([OP_I32_STORE8, 0x00, 0x00]) # store back in place
    body_8 += bytes([OP_LOCAL_GET, 0x03, OP_I32_CONST, 0x01, OP_I32_ADD, OP_LOCAL_SET, 0x03])
    body_8 += bytes([OP_BR, 0x00, OP_END, OP_END])

    # Step 3: compute final checksum in-place
    body_8 += bytes([OP_I64_CONST]) + encode_s64(FNV_OFFSET_BASIS) + bytes([OP_LOCAL_SET, 0x02])
    body_8 += bytes([OP_I32_CONST, 0x00, OP_LOCAL_SET, 0x03])
    body_8 += bytes([OP_BLOCK, TYPE_EMPTY, OP_LOOP, TYPE_EMPTY])
    body_8 += bytes([OP_LOCAL_GET, 0x03, OP_LOCAL_GET, 0x01, OP_I32_GE_U, OP_BR_IF, 0x01])
    body_8 += bytes([OP_LOCAL_GET, 0x05, OP_LOCAL_GET, 0x03, OP_I32_ADD, OP_I32_LOAD8_U, 0x00, 0x00])
    body_8 += bytes([OP_I64_EXTEND_I32_U, OP_LOCAL_GET, 0x02, OP_I64_XOR])
    body_8 += bytes([OP_I64_CONST]) + encode_s64(FNV_PRIME) + bytes([OP_I64_MUL, OP_LOCAL_SET, 0x02])
    body_8 += bytes([OP_LOCAL_GET, 0x03, OP_I32_CONST, 0x01, OP_I32_ADD, OP_LOCAL_SET, 0x03])
    body_8 += bytes([OP_BR, 0x00, OP_END, OP_END])

    # Update telemetry globals
    # total_ops += 1, zero_copy_ops += 1, bytes_processed += len, last_checksum = hash
    body_8 += bytes([OP_GLOBAL_GET, 0x02, OP_I64_CONST, 0x01, OP_I64_ADD, OP_GLOBAL_SET, 0x02])
    body_8 += bytes([OP_GLOBAL_GET, 0x04, OP_I64_CONST, 0x01, OP_I64_ADD, OP_GLOBAL_SET, 0x04])
    body_8 += bytes([OP_GLOBAL_GET, 0x05, OP_LOCAL_GET, 0x01, OP_I64_EXTEND_I32_U, OP_I64_ADD, OP_GLOBAL_SET, 0x05])
    body_8 += bytes([OP_LOCAL_GET, 0x02, OP_GLOBAL_SET, 0x06])

    # return final hash
    body_8 += bytes([OP_LOCAL_GET, 0x02, OP_END])
    code_bodies.append(encode_u32(len(body_8)) + body_8)

    # Fn 9: vxr_compute_checksum(ptr: i32, len: i32) -> i64
    # Param 0: ptr, Param 1: len
    # Local 2: hash (i64), Local 3: i (i32)
    body_9 = bytearray()
    body_9 += encode_u32(2)
    body_9 += encode_u32(1) + bytes([TYPE_I64]) # local 2: i64
    body_9 += encode_u32(1) + bytes([TYPE_I32]) # local 3: i32
    # hash = FNV_OFFSET_BASIS
    body_9 += bytes([OP_I64_CONST]) + encode_s64(FNV_OFFSET_BASIS) + bytes([OP_LOCAL_SET, 0x02])
    # i = 0
    body_9 += bytes([OP_I32_CONST, 0x00, OP_LOCAL_SET, 0x03])
    # block $b0
    body_9 += bytes([OP_BLOCK, TYPE_EMPTY])
    # loop $l0
    body_9 += bytes([OP_LOOP, TYPE_EMPTY])
    # if i >= len br 1 ($b0)
    body_9 += bytes([OP_LOCAL_GET, 0x03, OP_LOCAL_GET, 0x01, OP_I32_GE_U, OP_BR_IF, 0x01])
    # val = i32.load8_u (ptr + i)
    body_9 += bytes([OP_LOCAL_GET, 0x00, OP_LOCAL_GET, 0x03, OP_I32_ADD])
    body_9 += bytes([OP_I32_LOAD8_U, 0x00, 0x00]) # align 0, offset 0
    # hash = (hash ^ (i64.extend_i32_u val)) * FNV_PRIME
    body_9 += bytes([OP_I64_EXTEND_I32_U, OP_LOCAL_GET, 0x02, OP_I64_XOR])
    body_9 += bytes([OP_I64_CONST]) + encode_s64(FNV_PRIME) + bytes([OP_I64_MUL, OP_LOCAL_SET, 0x02])
    # i = i + 1
    body_9 += bytes([OP_LOCAL_GET, 0x03, OP_I32_CONST, 0x01, OP_I32_ADD, OP_LOCAL_SET, 0x03])
    body_9 += bytes([OP_BR, 0x00]) # loop again
    body_9 += bytes([OP_END, OP_END]) # end loop, end block
    body_9 += bytes([OP_LOCAL_GET, 0x02, OP_END]) # return hash
    code_bodies.append(encode_u32(len(body_9)) + body_9)

    # Fn 10: vxr_get_stats_total_ops() -> i64
    body_10 = bytes([0x00, OP_GLOBAL_GET, 0x02, OP_END])
    code_bodies.append(encode_u32(len(body_10)) + body_10)

    # Fn 11: vxr_get_stats_copy_ops() -> i64
    body_11 = bytes([0x00, OP_GLOBAL_GET, 0x03, OP_END])
    code_bodies.append(encode_u32(len(body_11)) + body_11)

    # Fn 12: vxr_get_stats_zero_copy_ops() -> i64
    body_12 = bytes([0x00, OP_GLOBAL_GET, 0x04, OP_END])
    code_bodies.append(encode_u32(len(body_12)) + body_12)

    # Fn 13: vxr_get_stats_bytes_processed() -> i64
    body_13 = bytes([0x00, OP_GLOBAL_GET, 0x05, OP_END])
    code_bodies.append(encode_u32(len(body_13)) + body_13)

    # Fn 14: vxr_get_stats_last_checksum() -> i64
    body_14 = bytes([0x00, OP_GLOBAL_GET, 0x06, OP_END])
    code_bodies.append(encode_u32(len(body_14)) + body_14)

    code_section = make_section(10, make_vector(code_bodies))

    # -------------------------------------------------------------------------
    # Assemble Final Module: Magic Header + Sections
    # -------------------------------------------------------------------------
    magic_header = b"\x00asm\x01\x00\x00\x00"
    return (
        magic_header +
        type_section +
        func_section +
        memory_section +
        global_section +
        export_section +
        code_section
    )


def main():
    root_dir = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
    out_dir = os.path.join(root_dir, "web", "static", "wasm")
    os.makedirs(out_dir, exist_ok=True)
    out_wasm = os.path.join(out_dir, "vxr_kernel.wasm")

    print("[VXR] Synthesizing standalone WebAssembly module...")
    wasm_bytes = build_wasm_binary()

    with open(out_wasm, "wb") as f:
        f.write(wasm_bytes)

    size = len(wasm_bytes)
    print(f"[VXR] Successfully generated: {out_wasm} ({size} bytes)")


if __name__ == "__main__":
    main()
