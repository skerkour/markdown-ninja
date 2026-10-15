package pingoo

import (
	"bytes"
	"context"
	cryptorand "crypto/rand"
	"fmt"
	"io"
	"os"
	"runtime"
	"time"

	"github.com/tetratelabs/wazero"
	wazeroapi "github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"markdown.ninja/pingoo-go/wasm"
	"markdown.ninja/stdx/retry"
)

// callWasmGuestFunction calls the given WASM function using JSON to serialize/deserialize input/output
func callWasmGuestFunction[I, O any](ctx context.Context, client *Client, functionName string, parameters I) (O, error) {
	logger := client.getLogger(ctx)
	logger.Debug("wasm: calling function: " + functionName)
	functionStartedAt := time.Now()

	// TODO: make this call able to handle concurrency without mutex.
	// For that, we first need to compile the WASM module to target `wasm32-wasip1-threds`.
	// Then we need to take in account wazero concurrency support (https://github.com/tetratelabs/wazero/issues/2217)
	// especially related to exported function calls (https://pkg.go.dev/github.com/tetratelabs/wazero/api#Function).
	// Unfortunately, as of now, we couldn't make it work. WASM calls don't work as if something in the stack
	// was not handling concurrency correctly, but we were unable to track down what.
	//
	// error calling wasm function: wasm error: out of bounds memory access
	//
	// Also, using the WASM memory module, which seems to be required to use WASM threads,
	// seems to lead to crashes in some Virtual Machines:
	// fatal error: runtime: out of memory
	//
	// The other option is to use a `sync.Pool` of wasm module, but then the modules need to be stateless
	// otherwise they will use too much memory (1 geoip database in memory per WASM module) and we need to
	// push more logic in the Go SDK instead of the WASM SDK.

	// var emptyOutput O
	// wasmModulePoolObject := client.wasmModulePool.Get()
	// if wasmModulePoolObject == nil {
	// 	err := errors.New("pingoo: error getting object from wasm sync.Pool. Object is nil")
	// 	return emptyOutput, err
	// }

	// wasmModule := wasmModulePoolObject.(*wasm.Module)
	// defer client.wasmModulePool.Put(wasmModule)
	wasmModule := client.wasmModule
	client.wasmModuleMutex.Lock()
	defer client.wasmModuleMutex.Unlock()

	output, err := wasm.CallGuestFunction[I, O](ctx, logger, wasmModule, functionName, parameters)
	if err != nil {
		logger.Debug("wasm: function " + functionName + " completed in " + time.Since(functionStartedAt).String() + " with error: " + err.Error())
		return output, err
	}

	logger.Debug("wasm: function " + functionName + " successfully completed in " + time.Since(functionStartedAt).String())

	return output, nil
}

func (client *Client) refreshPingooWasm(ctx context.Context) (err error) {
	logger := client.getLogger(ctx)
	logger.Debug("pingoo: starting pingoo.wasm refresh")

	currentPingooWasmEtag := client.wasmEtag.Load()
	if currentPingooWasmEtag == nil {
		currentPingooWasmEtag = new("")
	}

	pingooWasmRes, err := client.DownloadPingooWasm(ctx, *currentPingooWasmEtag)
	if err != nil {
		return fmt.Errorf("pingoo: error downloading pingoo.wasm %w", err)
	}
	defer pingooWasmRes.Data.Close()

	if pingooWasmRes.NotModified || pingooWasmRes.Etag == *currentPingooWasmEtag && pingooWasmRes.Etag != "" {
		logger.Debug("pingoo: no new pingoo.wasm available")
		return nil
	}

	// download the actual wasm in a buffer
	pingooWasmBuffer := bytes.NewBuffer(make([]byte, 0, 2_000_000))
	_, err = io.Copy(pingooWasmBuffer, pingooWasmRes.Data)
	if err != nil && err != io.EOF {
		return fmt.Errorf("pingoo: error downloading pingoo.wasm: %w", err)
	}
	err = nil

	client.wasmModuleMutex.Lock()
	defer client.wasmModuleMutex.Unlock()

	wasmCtx := context.Background()

	// See https://github.com/tetratelabs/wazero/issues/2156
	// and https://github.com/wasilibs/go-re2/blob/main/internal/re2_wazero.go
	// for imformation about how to configure wazero to use a WASM lib using WASM memory
	// wasmCtx = experimental.WithMemoryAllocator(wasmCtx, wazeroallocator.NewNonMoving())

	// More wazero docs:
	// How to use HostFunctionBuilder with multiple goroutines? https://github.com/tetratelabs/wazero/issues/2217
	// Clarification on concurrency semantics for invocations https://github.com/tetratelabs/wazero/issues/2292
	// Improve InstantiateModule concurrency performance https://github.com/tetratelabs/wazero/issues/602
	// Add option to change Memory capacity https://github.com/tetratelabs/wazero/issues/500
	// Document best practices around invoking a wasi module multiple times https://github.com/tetratelabs/wazero/issues/985
	// API shape https://github.com/tetratelabs/wazero/issues/425

	client.wasmRuntime = wazero.NewRuntimeWithConfig(wasmCtx, wazero.NewRuntimeConfigCompiler().WithCoreFeatures(wazeroapi.CoreFeaturesV2|experimental.CoreFeaturesThreads).WithMemoryLimitPages(65536))

	wasi_snapshot_preview1.MustInstantiate(wasmCtx, client.wasmRuntime)

	// enabling WASM memory leads to crashes in some Virtual Machines such as firecracker:
	// This error may come from the fact that the VM has less than 4GB of memory, but the memory module
	// tries to "reserve" 4GB of memory.
	//
	// fatal error: runtime: out of memory
	// github.com/tetratelabs/wazero@v1.9.0/runtime.go:302
	// ...
	// github.com/tetratelabs/wazero@v1.9.0/internal/wasm/memory.go:92
	// _, err = client.wasmRuntime.InstantiateWithConfig(wasmCtx, assets.MemoryWasm, wazero.NewModuleConfig().WithName("env"))
	// if err != nil {
	// 	return nil, fmt.Errorf("pingoo: error instantiating wasm memory module: %w", err)
	// }

	// _, err = client.wasmRuntime.NewHostModuleBuilder("host").
	// 	NewFunctionBuilder().WithFunc(func(ctx context.Context, input wasm.Buffer) wasm.Buffer {
	// 	return wasm.HandleHostFunctionCall(ctx, client.resolveHostForIp, input)
	// }).Export("dns_lookup_ip_address").
	// 	Instantiate(wasmCtx)
	// if err != nil {
	// 	return fmt.Errorf("pingoo: error instantiating wasm host module (host): %w", err)
	// }

	compiledWasmModule, err := client.wasmRuntime.CompileModule(wasmCtx, pingooWasmBuffer.Bytes())
	if err != nil {
		return fmt.Errorf("pingoo: error compiling wasm pingoo module: %w", err)
	}

	instantiatedWasmModule, err := client.wasmRuntime.InstantiateModule(wasmCtx, compiledWasmModule, wazero.NewModuleConfig().
		WithStartFunctions("_initialize").WithSysNanosleep().WithSysNanotime().WithSysWalltime().WithName("").WithRandSource(cryptorand.Reader).WithStdout(os.Stdout).WithStderr(os.Stderr),
	// for debugging
	// .WithStdout(os.Stdout).WithStderr(os.Stderr),
	)
	if err != nil {
		return fmt.Errorf("pingoo: error instantiating WASM module: %w", err)
	}

	client.wasmModule, err = wasm.NewModule(instantiatedWasmModule)
	if err != nil {
		return fmt.Errorf("pingoo: error instantiating WASM module: %w", err)
	}

	runtime.SetFinalizer(client.wasmModule, func(module *wasm.Module) {
		module.Close(wasmCtx)
	})

	// as recommended in https://github.com/tetratelabs/wazero/issues/2217
	// we use a sync.Pool of wasm modules in order to handle concurrent calls of WASM functions
	// client.wasmModulePool = &sync.Pool{
	// 	New: func() any {
	// 		// poolObjectCtx := context.Background()
	// 		instantiatedWasmModule, err := client.wasmRuntime.InstantiateModule(wasmCtx, client.compiledWasmModule, wazero.NewModuleConfig().
	// 			WithStartFunctions("_initialize").WithSysNanosleep().WithSysNanotime().WithSysWalltime().WithName("").WithRandSource(cryptorand.Reader).WithStdout(os.Stdout).WithStderr(os.Stderr),
	// 		// for debugging
	// 		// .WithStdout(os.Stdout).WithStderr(os.Stderr),
	// 		)
	// 		if err != nil {
	// 			logger.Error("waf.wasmModulePool.New: error instantiating WASM module", slogx.Err(err))
	// 			return nil
	// 		}

	// 		poolObject, err := wasm.NewModule(instantiatedWasmModule)
	// 		if err != nil {
	// 			logger.Error("waf.wasmModulePool.New: error instantiating WASM module", slogx.Err(err))
	// 			return nil
	// 		}

	// 		// use a finalizer to Close the module, as recommended in https://github.com/golang/go/issues/23216
	// 		runtime.SetFinalizer(poolObject, func(module *wasm.Module) {
	// 			module.Close(wasmCtx)
	// 		})

	// 		return poolObject
	// 	},
	// }

	client.wasmEtag.Store(&pingooWasmRes.Etag)

	logger.Debug("pingoo: pingoo.wasm successfully downloaded")

	return nil
}

func (client *Client) refreshPingooWasmInBackground(ctx context.Context) {
	logger := client.getLogger(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(client.wasmRefreshInterval):
		}

		err := retry.Do(func() error {
			retryErr := client.refreshPingooWasm(ctx)
			return retryErr
		}, retry.Context(ctx), retry.Attempts(5), retry.Delay(time.Second), retry.DelayType(retry.FixedDelay))
		if err != nil {
			logger.Warn("pingoo: error refreshing pingoo.wasm: " + err.Error())
		}
	}
}
