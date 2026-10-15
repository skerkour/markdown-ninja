package riscvemu

import (
	"fmt"
	"os"
	"testing"
)

func TestDebugCertFault(t *testing.T) {
	data, err := os.ReadFile("/tmp/hw2/httpsget")
	if err != nil {
		t.Skip("no httpsget")
	}
	prog, mem, entry, err := LoadELF(data)
	if err != nil {
		t.Fatal(err)
	}
	// record rejects to find the faulting slot
	prog.recordRejects = true
	machine := NewMachine(prog, mem)
	sp, auxv := buildStack(mem, Options{})
	machine.auxvBytes = encodeAuxv(auxv)
	machine.stackStart = sp
	th := machine.newThread(prog.addrsToSlot(entry))
	th.x[2] = sp
	_, err = machine.Schedule()
	if err != nil {
		if f, ok := err.(*Fault); ok {
			fmt.Fprintf(os.Stderr, "FAULT pc=0x%x msg=%s\n", f.PC, f.Msg)
			off := int(f.PC - prog.base)
			seg := readSeg(t, data)
			if off+4 <= len(seg) {
				lo := uint16(seg[off]) | uint16(seg[off+1])<<8
				fmt.Fprintf(os.Stderr, "  inst16=0x%04x\n", lo)
			}
		}
		return
	}
	t.Log("no fault")
}

func readSeg(t *testing.T, data []byte) []byte {
	phoff := uint64(data[32]) | uint64(data[33])<<8 | uint64(data[34])<<16 | uint64(data[35])<<24 |
		uint64(data[36])<<32 | uint64(data[37])<<40 | uint64(data[38])<<48 | uint64(data[39])<<56
	phentsz := uint16(data[54]) | uint16(data[55])<<8
	phnum := uint16(data[56]) | uint16(data[57])<<8
	for i := 0; i < int(phnum); i++ {
		off := phoff + uint64(i)*uint64(phentsz)
		pOffset := uint64(data[off+8]) | uint64(data[off+9])<<8 | uint64(data[off+10])<<16 | uint64(data[off+11])<<24 |
			uint64(data[off+12])<<32 | uint64(data[off+13])<<40 | uint64(data[off+14])<<48 | uint64(data[off+15])<<56
		pFilesz := uint64(data[off+32]) | uint64(data[off+33])<<8 | uint64(data[off+34])<<16 | uint64(data[off+35])<<24 |
			uint64(data[off+36])<<32 | uint64(data[off+37])<<40 | uint64(data[off+38])<<48 | uint64(data[off+39])<<56
		flags := uint32(data[off+4]) | uint32(data[off+5])<<8 | uint32(data[off+6])<<16 | uint32(data[off+7])<<24
		if flags&1 != 0 {
			return data[pOffset : pOffset+pFilesz]
		}
	}
	return nil
}
