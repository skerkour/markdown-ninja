package riscvemu

import (
	"fmt"
	"os"
	"testing"
	"time"
)

func TestDebugPreempt(t *testing.T) {
	data, err := os.ReadFile("/tmp/hw2/spin4")
	if err != nil {
		t.Skip("no spin4")
	}
	prog, mem, entry, err := LoadELF(data)
	if err != nil {
		t.Fatal(err)
	}
	machine := NewMachine(prog, mem)
	machine.traceSys = true
	machine.tracePreempt = true
	machine.tracePreemptTid = 1000
	sp, auxv := buildStack(mem, Options{})
	machine.auxvBytes = encodeAuxv(auxv)
	machine.stackStart = sp
	th := machine.newThread(prog.addrsToSlot(entry))
	th.x[2] = sp

	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			case <-time.After(3 * time.Second):
				line := fmt.Sprintf("threads=%d", len(machine.threads))
				for _, t := range machine.threads {
					st := map[int]string{0: "r", 1: "f", 2: "s", 3: "x"}[t.state]
					line += fmt.Sprintf(" t%d:%s@0x%x", t.tid, st, machine.prog.addr(t.pc))
				}
				fmt.Fprintf(os.Stderr, "%s\n", line)
			}
		}
	}()
	code, err := machine.Schedule()
	close(done)
	if err != nil {
		t.Logf("error: %v", err)
		return
	}
	t.Logf("exit=%d", code)
}
