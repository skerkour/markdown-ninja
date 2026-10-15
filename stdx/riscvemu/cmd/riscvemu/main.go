// Command riscvemu runs a riscv64 Linux ELF binary (e.g. a Go program
// cross-compiled with GOARCH=riscv64) on the interpreter and forwards the
// guest's stdout/stderr and exit code to the host.
//
// Usage:
//
//	riscvemu <binary> [args...]
package main

import (
	"fmt"
	"os"

	"markdown.ninja/stdx/riscvemu"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: riscvemu <binary> [args...]")
		os.Exit(2)
	}

	code, err := riscvemu.RunELFFile(os.Args[1], riscvemu.Options{
		Args: os.Args[2:],
		Env:  os.Environ(),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "riscvemu: %v\n", err)
		os.Exit(1)
	}
	os.Exit(code)
}
