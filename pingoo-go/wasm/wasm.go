package wasm

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/fxamacker/cbor/v2"
	wazeroapi "github.com/tetratelabs/wazero/api"
	"markdown.ninja/stdx/log/slogx"
)

type ModuleCtxKeyType struct{}

// ModuleCtxKeyType is the key that holds the wasmModule in a host WASM function call.
var ModuleCtxKey = ModuleCtxKeyType{}

type Module struct {
	module       wazeroapi.Module
	callFunction wazeroapi.Function
	allocate     wazeroapi.Function
	deallocate   wazeroapi.Function
}

type Empty struct{}

func (module *Module) Close(ctx context.Context) {
	module.module.Close(ctx)
}

func NewModule(module wazeroapi.Module) (ret *Module, err error) {
	ret = &Module{
		module:       module,
		callFunction: module.ExportedFunction("call_function"),
		allocate:     module.ExportedFunction("allocate"),
		deallocate:   module.ExportedFunction("deallocate"),
	}
	if ret.callFunction == nil {
		return nil, errors.New("wasm module is not valid. exported function call_function is missing")
	}
	if ret.allocate == nil {
		return nil, errors.New("wasm module is not valid. exported function allocate is missing")
	}
	if ret.deallocate == nil {
		return nil, errors.New("wasm module is not valid. exported function deallocate is missing")
	}

	return ret, nil
}

// Buffer represents a pointer to a buffer allocated in a WASM module's memory
// Because we pack the pointer with its length, it currently only supports wasm32
// [ pointer (32 bits) | length (32 bits)]
type Buffer uint64

func (buffer Buffer) Pointer() uint32 {
	return uint32(buffer >> 32)
}

func (buffer Buffer) Size() uint32 {
	return uint32(buffer)
}

func NewBuffer(pointer, length uint32) Buffer {
	return Buffer((uint64(pointer) << 32) | uint64(length))
}

type Request[P any] struct {
	Function   string `json:"function" cbor:"function"`
	Parameters P      `json:"parameters" cbor:"parameters"`
}

type Result[T any] struct {
	Ok    *T     `json:"ok,omitempty" cbor:"ok,omitempty"`
	Error *Error `json:"error,omitempty" cbor:"error,omitempty"`
}

type Error struct {
	Message string `json:"message" cbor:"message"`
}

// func HandleHostFunctionCall[I, O any](ctx context.Context, module wazeroapi.Module, input I) (output O, err error) {
// 	var ret O

// 	return ret, nil
// }

// CallFunction calls the given WASM function using JSON to serialize/deserialize input/output
func CallGuestFunction[I, O any](ctx context.Context, logger *slog.Logger, wasmModule *Module, functionName string, parameters I) (O, error) {
	var emptyOutput O

	ctx = context.WithValue(ctx, ModuleCtxKey, wasmModule)

	input := Request[I]{
		Function:   functionName,
		Parameters: parameters,
	}

	// callFunction := wasmModule.module.ExportedFunction("call_function")
	// allocate := wasmModule.module.ExportedFunction("allocate")
	// deallocate := wasmModule.module.ExportedFunction("deallocate")
	callFunction := wasmModule.callFunction
	allocate := wasmModule.allocate
	deallocate := wasmModule.deallocate

	// first we serialize the input ot JSON
	// then we allocate WASM memory for this JSON using the module's exported alloc function.
	// Don't forget to free the WASM input buffer
	// then we copy the input JSON into the WASM memoy
	// then we call the WASM function and pass it a pointer to the buffer that we have allocated
	// the function returns a pointer to a buffer it has allocated containing the output
	// then we read the output buffer from WASM's memory to the host (Go) memory and free the WASM ouput buffer
	// then we deserialize the output buffer content from JSON

	// serialize input to JSON
	inputBytes, err := cbor.Marshal(input)
	if err != nil {
		return emptyOutput, fmt.Errorf("error marshalling input data to JSON: %w", err)
	}

	// allocate WASM memory for input data
	allocateInputResults, err := allocate.Call(ctx, uint64(len(inputBytes)))
	if err != nil {
		return emptyOutput, fmt.Errorf("error allocating wasm memory for function call input: %w", err)
	}

	wasmInputBuffer := Buffer(allocateInputResults[0])
	defer func() {
		// this memory was allocated by the WASM module so we have to deallocate it when finished
		if _, deallocateErr := deallocate.Call(ctx, uint64(wasmInputBuffer)); deallocateErr != nil {
			logger.Error("error deallocating wasm memory for function call input", slogx.Err(deallocateErr))
		}
	}()

	// write serialized input data into WASM's memory
	if !wasmModule.module.Memory().Write(wasmInputBuffer.Pointer(), inputBytes) {
		return emptyOutput, fmt.Errorf("error writing function call input data to wasm memory: Memory.Write(%d, %d) out of range of memory size %d",
			wasmInputBuffer.Pointer(), wasmInputBuffer.Size(), wasmModule.module.Memory().Size())
	}

	// call WASM function
	wasmResults, err := callFunction.Call(ctx, uint64(wasmInputBuffer))
	if err != nil {
		return emptyOutput, fmt.Errorf("error calling wasm function: %w", err)
	}

	// data returned from WASM's side is returned as an allocated buffer which (pointer, length) pair that is packed
	// into an uint64. It needs to be freed after having been read.
	wasmOutputBuffer := Buffer(wasmResults[0])
	defer func() {
		if _, deallocateErr := deallocate.Call(ctx, uint64(wasmOutputBuffer)); deallocateErr != nil {
			logger.Error("error deallocating wasm memory for function call output", slogx.Err(deallocateErr))
		}
	}()

	// read serialized output data from WASM memory
	outputBytes, outputReadOk := wasmModule.module.Memory().Read(wasmOutputBuffer.Pointer(), wasmOutputBuffer.Size())
	if !outputReadOk {
		return emptyOutput, fmt.Errorf("error reading function call output data from wasm memory: Memory.Read(%d, %d) out of range of memory size %d",
			wasmOutputBuffer.Pointer(), wasmOutputBuffer.Size(), wasmModule.module.Memory().Size())
	}

	var wasmResult Result[O]
	err = cbor.Unmarshal(outputBytes, &wasmResult)
	if err != nil {
		return emptyOutput, fmt.Errorf("error unmarshalling JSON output: %w", err)
	}

	if wasmResult.Error != nil {
		return emptyOutput, errors.New(wasmResult.Error.Message)
	}

	return *wasmResult.Ok, nil
}

func HandleHostFunctionCall[I, O any](ctx context.Context, logger *slog.Logger, hostFunction func(context.Context, I) (O, error), inputBuffer Buffer) Buffer {
	wasmModule := ctx.Value(ModuleCtxKey).(*Module)

	// allocate := wasmModule.module.ExportedFunction("allocate")
	// deallocate := wasmModule.module.ExportedFunction("deallocate")
	allocate := wasmModule.allocate
	deallocate := wasmModule.deallocate

	inputBytes, readInputIp := wasmModule.module.Memory().Read(inputBuffer.Pointer(), inputBuffer.Size())
	if !readInputIp {
		return newWasmError(ctx,
			fmt.Errorf("error reading host function call input data from wasm memory: Memory.Read(%d, %d) out of range of memory size %d",
				inputBuffer.Pointer(), inputBuffer.Size(), wasmModule.module.Memory().Size()),
			wasmModule, allocate, deallocate)
	}

	var input I
	err := cbor.Unmarshal(inputBytes, &input)
	if err != nil {
		return newWasmError(ctx,
			fmt.Errorf("error unmarshalling host function call input data from JSON: %w", err),
			wasmModule,
			allocate,
			deallocate,
		)
	}

	output, err := hostFunction(ctx, input)
	if err != nil {
		logger.Warn(err.Error())
		return newWasmError(ctx, err, wasmModule, allocate, deallocate)
	}
	outputResult := Result[O]{
		Ok: &output,
	}
	outputBytes, err := cbor.Marshal(outputResult)
	if err != nil {
		return newWasmError(ctx,
			fmt.Errorf("error marshalling host function call output data to JSON: %w", err),
			wasmModule,
			allocate,
			deallocate,
		)
	}

	allocateOutputRes, err := allocate.Call(ctx, uint64(len(outputBytes)))
	if err != nil {
		return newWasmError(ctx,
			fmt.Errorf("error allocating memory for host function call output data: %w", err),
			wasmModule,
			allocate,
			deallocate)
	}
	wasmOutputBuffer := Buffer(allocateOutputRes[0])

	if !wasmModule.module.Memory().Write(wasmOutputBuffer.Pointer(), outputBytes) {
		// TODO: log error?

		// deallocate := wazeroModule.ExportedFunction("deallocate")
		if _, deallocateErr := deallocate.Call(ctx, uint64(wasmOutputBuffer)); deallocateErr != nil {
			logger.Error("error deallocating wasm memory for host function call output", slogx.Err(deallocateErr))
		}

		return newWasmError(ctx,
			fmt.Errorf("error writing host function call output data to wasm memory: Memory.Write(%d, %d) out of range of memory size %d",
				wasmOutputBuffer.Pointer(), wasmOutputBuffer.Size(), wasmModule.module.Memory().Size()),
			wasmModule,
			allocate,
			deallocate)
	}

	return wasmOutputBuffer
}

func newWasmError(ctx context.Context, err error, wasmModule *Module, allocate wazeroapi.Function, deallocate wazeroapi.Function) Buffer {
	wasmErr := Result[Empty]{
		Error: &Error{
			Message: err.Error(),
		},
	}
	outputBytes, err := cbor.Marshal(wasmErr)
	if err != nil {
		// TODO: log error?
		return Buffer(0)
	}

	allocateOutputRes, err := allocate.Call(ctx, uint64(len(outputBytes)))
	if err != nil {
		// TODO: log error?
		return Buffer(0)
	}
	wasmOutputBuffer := Buffer(allocateOutputRes[0])

	if !wasmModule.module.Memory().Write(wasmOutputBuffer.Pointer(), outputBytes) {
		// TODO: log error?

		// deallocate := wazeroModule.ExportedFunction("deallocate")
		if _, deallocateErr := deallocate.Call(ctx, uint64(wasmOutputBuffer)); deallocateErr != nil {
			// TODO: log error?
			// waf.logger.Error("error deallocating wasm memory for host function call output", slogx.Err(deallocateErr))
		}

		return Buffer(0)
	}

	return wasmOutputBuffer
}
