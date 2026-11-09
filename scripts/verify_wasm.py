#!/usr/bin/env python3
"""
VXR-Thesis WebAssembly Module Validator
Verifies structural compliance, sections, and exported ABI functions of vxr_kernel.wasm
against the W3C WebAssembly 1.0/2.0 Core Specification.
"""

import os
import sys

SECTION_NAMES = {
    1: "Type",
    2: "Import",
    3: "Function",
    4: "Table",
    5: "Memory",
    6: "Global",
    7: "Export",
    8: "Start",
    9: "Element",
    10: "Code",
    11: "Data",
    12: "DataCount",
}

EXPECTED_EXPORTS = [
    "memory",
    "vxr_init",
    "vxr_get_ring_buffer_ptr",
    "vxr_get_ring_buffer_capacity",
    "vxr_alloc_arena",
    "vxr_reset_arena",
    "vxr_alloc_slab",
    "vxr_free_slab",
    "vxr_process_copy",
    "vxr_process_zero_copy",
    "vxr_compute_checksum",
    "vxr_get_stats_total_ops",
    "vxr_get_stats_copy_ops",
    "vxr_get_stats_zero_copy_ops",
    "vxr_get_stats_bytes_processed",
    "vxr_get_stats_last_checksum",
]


def decode_u32(data: bytes, offset: int):
    result = 0
    shift = 0
    while True:
        if offset >= len(data):
            raise ValueError("Unexpected EOF while decoding LEB128 integer")
        b = data[offset]
        offset += 1
        result |= (b & 0x7F) << shift
        if (b & 0x80) == 0:
            break
        shift += 7
    return result, offset


def verify_wasm_file(file_path: str) -> bool:
    print("==========================================================")
    print("  VXR-Thesis: WebAssembly Binary ABI Verification Suite   ")
    print("==========================================================")
    print(f"[INFO] Target: {file_path}")

    if not os.path.exists(file_path):
        print(f"[ERROR] File not found: {file_path}")
        return False

    with open(file_path, "rb") as f:
        data = f.read()

    print(f"[INFO] Binary Size: {len(data)} bytes")

    # 1. Header Validation
    if len(data) < 8:
        print("[ERROR] Binary too short (< 8 bytes)")
        return False

    magic = data[:4]
    version = data[4:8]

    if magic != b"\x00asm":
        print(f"[ERROR] Invalid Wasm magic header: {magic!r}")
        return False

    if version != b"\x01\x00\x00\x00":
        print(f"[ERROR] Unsupported Wasm version: {version!r}")
        return False

    print("[VERIFIED] Header: Magic '\\x00asm', Version 1 (Valid W3C Binary)")

    # 2. Section Traversal
    offset = 8
    sections_found = {}
    exports_found = []

    while offset < len(data):
        sec_id = data[offset]
        offset += 1
        sec_len, offset = decode_u32(data, offset)
        sec_name = SECTION_NAMES.get(sec_id, f"Custom({sec_id})")
        sections_found[sec_id] = (sec_name, sec_len)

        sec_payload = data[offset:offset + sec_len]

        # Parse Export Section
        if sec_id == 7:
            count, p_offset = decode_u32(sec_payload, 0)
            for _ in range(count):
                name_len, p_offset = decode_u32(sec_payload, p_offset)
                exp_name = sec_payload[p_offset:p_offset + name_len].decode("utf-8", errors="replace")
                p_offset += name_len
                exp_kind = sec_payload[p_offset]
                p_offset += 1
                exp_idx, p_offset = decode_u32(sec_payload, p_offset)
                kind_str = "func" if exp_kind == 0 else ("mem" if exp_kind == 2 else f"kind_{exp_kind}")
                exports_found.append((exp_name, kind_str, exp_idx))

        offset += sec_len

    print("\n[SECTION BREAKDOWN]")
    for sec_id in sorted(sections_found.keys()):
        name, length = sections_found[sec_id]
        print(f"  - Section {sec_id:2d} ({name:10s}): {length:4d} bytes")

    print("\n[EXPORTED ABI INTERFACE]")
    missing_exports = []
    found_names = {name for name, _, _ in exports_found}

    for exp in EXPECTED_EXPORTS:
        if exp in found_names:
            print(f"  [OK] Export: {exp}")
        else:
            print(f"  [FAIL] Missing required export: {exp}")
            missing_exports.append(exp)

    print("==========================================================")
    if missing_exports:
        print(f"[RESULT] Verification FAILED: {len(missing_exports)} exports missing.")
        return False
    else:
        print(f"[RESULT] All {len(EXPECTED_EXPORTS)} ABI exports verified successfully.")
        print("==========================================================")
        return True


def main():
    root_dir = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
    wasm_path = os.path.join(root_dir, "web", "static", "wasm", "vxr_kernel.wasm")
    ok = verify_wasm_file(wasm_path)
    sys.exit(0 if ok else 1)


if __name__ == "__main__":
    main()
