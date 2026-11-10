package engine

import (
	"context"
	"fmt"
	"os"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// WasmInstance wraps the compiled wazero runtime module and function exports.
type WasmInstance struct {
	runtime        wazero.Runtime
	module         api.Module
	memory         api.Memory
	ringBufferPtr  uint32
	ringCapacity   uint32

	// Exported ABI function bindings
	fnInit               api.Function
	fnGetRingBufferPtr   api.Function
	fnGetRingCapacity    api.Function
	fnAllocArena         api.Function
	fnResetArena         api.Function
	fnAllocSlab          api.Function
	fnFreeSlab           api.Function
	fnProcessCopy        api.Function
	fnProcessZeroCopy    api.Function
	fnComputeChecksum    api.Function
	fnGetTotalOps        api.Function
	fnGetCopyOps         api.Function
	fnGetZeroCopyOps     api.Function
	fnGetBytesProcessed  api.Function
	fnGetLastChecksum    api.Function
}

// LoadWasmKernel reads the Wasm binary, instantiates the wazero runtime, and verifies ABI exports.
func LoadWasmKernel(ctx context.Context, wasmPath string) (*WasmInstance, error) {
	wasmBytes, err := os.ReadFile(wasmPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read WebAssembly binary at %s: %w", wasmPath, err)
	}

	// Create pure-Go wazero runtime with memory allocation controls
	r := wazero.NewRuntime(ctx)

	// Compile module
	compiled, err := r.CompileModule(ctx, wasmBytes)
	if err != nil {
		r.Close(ctx)
		return nil, fmt.Errorf("failed to compile Wasm module: %w", err)
	}

	// Instantiate module with standard configuration
	config := wazero.NewModuleConfig().WithName("vxr_kernel")
	mod, err := r.InstantiateModule(ctx, compiled, config)
	if err != nil {
		r.Close(ctx)
		return nil, fmt.Errorf("failed to instantiate Wasm module: %w", err)
	}

	mem := mod.Memory()
	if mem == nil {
		r.Close(ctx)
		return nil, fmt.Errorf("wasm module does not export linear memory")
	}

	inst := &WasmInstance{
		runtime: r,
		module:  mod,
		memory:  mem,
	}

	// Bind exported functions
	bind := func(name string) (api.Function, error) {
		fn := mod.ExportedFunction(name)
		if fn == nil {
			return nil, fmt.Errorf("required ABI export %q not found in Wasm module", name)
		}
		return fn, nil
	}

	var bindErr error
	if inst.fnInit, bindErr = bind("vxr_init"); bindErr != nil { return nil, bindErr }
	if inst.fnGetRingBufferPtr, bindErr = bind("vxr_get_ring_buffer_ptr"); bindErr != nil { return nil, bindErr }
	if inst.fnGetRingCapacity, bindErr = bind("vxr_get_ring_buffer_capacity"); bindErr != nil { return nil, bindErr }
	if inst.fnAllocArena, bindErr = bind("vxr_alloc_arena"); bindErr != nil { return nil, bindErr }
	if inst.fnResetArena, bindErr = bind("vxr_reset_arena"); bindErr != nil { return nil, bindErr }
	if inst.fnAllocSlab, bindErr = bind("vxr_alloc_slab"); bindErr != nil { return nil, bindErr }
	if inst.fnFreeSlab, bindErr = bind("vxr_free_slab"); bindErr != nil { return nil, bindErr }
	if inst.fnProcessCopy, bindErr = bind("vxr_process_copy"); bindErr != nil { return nil, bindErr }
	if inst.fnProcessZeroCopy, bindErr = bind("vxr_process_zero_copy"); bindErr != nil { return nil, bindErr }
	if inst.fnComputeChecksum, bindErr = bind("vxr_compute_checksum"); bindErr != nil { return nil, bindErr }
	if inst.fnGetTotalOps, bindErr = bind("vxr_get_stats_total_ops"); bindErr != nil { return nil, bindErr }
	if inst.fnGetCopyOps, bindErr = bind("vxr_get_stats_copy_ops"); bindErr != nil { return nil, bindErr }
	if inst.fnGetZeroCopyOps, bindErr = bind("vxr_get_stats_zero_copy_ops"); bindErr != nil { return nil, bindErr }
	if inst.fnGetBytesProcessed, bindErr = bind("vxr_get_stats_bytes_processed"); bindErr != nil { return nil, bindErr }
	if inst.fnGetLastChecksum, bindErr = bind("vxr_get_stats_last_checksum"); bindErr != nil { return nil, bindErr }

	// Initialize the guest kernel memory partitions:
	// Ring: 256KB, Arena: 512KB, Slab: 256KB, Chunk: 256B
	_, err = inst.fnInit.Call(ctx, 256*1024, 512*1024, 256*1024, 256)
	if err != nil {
		r.Close(ctx)
		return nil, fmt.Errorf("vxr_init call failed: %w", err)
	}

	// Cache ring buffer location and capacity
	resPtr, err := inst.fnGetRingBufferPtr.Call(ctx)
	if err != nil || len(resPtr) == 0 {
		r.Close(ctx)
		return nil, fmt.Errorf("failed to query ring buffer pointer: %w", err)
	}
	inst.ringBufferPtr = uint32(resPtr[0])

	resCap, err := inst.fnGetRingCapacity.Call(ctx)
	if err != nil || len(resCap) == 0 {
		r.Close(ctx)
		return nil, fmt.Errorf("failed to query ring buffer capacity: %w", err)
	}
	inst.ringCapacity = uint32(resCap[0])

	return inst, nil
}

// Close releases the wazero runtime and resources.
func (w *WasmInstance) Close(ctx context.Context) error {
	return w.runtime.Close(ctx)
}

// RingBufferPtr returns the guest linear memory base offset of the zero-copy buffer.
func (w *WasmInstance) RingBufferPtr() uint32 {
	return w.ringBufferPtr
}

// RingCapacity returns the capacity in bytes of the zero-copy buffer.
func (w *WasmInstance) RingCapacity() uint32 {
	return w.ringCapacity
}

// Memory returns direct access to guest linear memory.
func (w *WasmInstance) Memory() api.Memory {
	return w.memory
}
