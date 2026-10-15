package riscvemu

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// buildHello builds a riscv64 linux hello-world binary for testing.
func buildHello(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "hello.go")
	os.WriteFile(src, []byte(`package main

import "fmt"

func main() {
	fmt.Println("Hello, World!")
}
`), 0644)

	bin := filepath.Join(dir, "hello")
	cmd := exec.Command("go", "build", "-o", bin, src)
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=riscv64", "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("cross-compile failed: %v\n%s", err, out)
	}
	return bin
}

func TestHelloWorld(t *testing.T) {
	bin := buildHello(t)
	data, err := os.ReadFile(bin)
	if err != nil {
		t.Fatal(err)
	}
	code, err := RunELF(data, Options{})
	if err != nil {
		t.Fatalf("emulator error: %v", err)
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
}

func TestRunELFFile(t *testing.T) {
	bin := buildHello(t)
	code, err := RunELFFile(bin, Options{})
	if err != nil {
		t.Fatalf("emulator error: %v", err)
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
}
