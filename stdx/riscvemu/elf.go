package riscvemu

import (
	"encoding/binary"
	"fmt"
	"os"
)

// elf64 program header constants
const (
	ptLoad  = 1
	pfX     = 1
	pfW     = 2
	pfR     = 4
	elfBits = 64
)

// LoadELF parses a riscv64 ELF executable, loads segments into memory and
// compiles the executable text segment into a closure program.
func LoadELF(data []byte) (*program, *Mem, uint64, error) {
	if len(data) < 64 || data[0] != 0x7f || data[1] != 'E' || data[2] != 'L' || data[3] != 'F' {
		return nil, nil, 0, fmt.Errorf("not an ELF file")
	}
	if data[4] != 2 { // ELF64
		return nil, nil, 0, fmt.Errorf("not a 64-bit ELF")
	}
	if data[5] != 1 { // little-endian
		return nil, nil, 0, fmt.Errorf("not little-endian")
	}
	machine := binary.LittleEndian.Uint16(data[18:20])
	if machine != 243 { // EM_RISCV
		return nil, nil, 0, fmt.Errorf("not a RISC-V binary (machine %d)", machine)
	}
	entry := binary.LittleEndian.Uint64(data[24:32])
	phoff := binary.LittleEndian.Uint64(data[32:40])
	phentsz := binary.LittleEndian.Uint16(data[54:56])
	phnum := binary.LittleEndian.Uint16(data[56:58])

	mem := NewMem()

	var textSeg []byte
	var textBase uint64

	for i := 0; i < int(phnum); i++ {
		off := phoff + uint64(i)*uint64(phentsz)
		if off+56 > uint64(len(data)) {
			break
		}
		ptype := binary.LittleEndian.Uint32(data[off : off+4])
		if ptype != ptLoad {
			continue
		}
		pOffset := binary.LittleEndian.Uint64(data[off+8 : off+16])
		pVaddr := binary.LittleEndian.Uint64(data[off+16 : off+24])
		pFilesz := binary.LittleEndian.Uint64(data[off+32 : off+40])
		flags := binary.LittleEndian.Uint32(data[off+4 : off+8])

		// load file content
		if pOffset+pFilesz <= uint64(len(data)) {
			mem.Write(pVaddr, data[pOffset:pOffset+pFilesz])
		}
		// zero-fill the rest (already zero pages)

		if flags&pfX != 0 && textSeg == nil {
			textSeg = data[pOffset : pOffset+pFilesz]
			textBase = pVaddr
		}
	}

	if textSeg == nil {
		return nil, nil, 0, fmt.Errorf("no executable segment")
	}

	prog := compile(textSeg, textBase)
	return prog, mem, entry, nil
}

// LoadELFFile loads an ELF executable from disk.
func LoadELFFile(path string) (*program, *Mem, uint64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, 0, err
	}
	return LoadELF(data)
}

// ELF helper: machine code for a riscv64 linux binary.
const (
	atNull   = 0
	atPhdr   = 3
	atPhent  = 4
	atPhnum  = 5
	atPagesz = 6
	atBase   = 7
	atFlags  = 8
	atEntry  = 9
	atUid    = 11
	atEuid   = 12
	atGid    = 13
	atEgid   = 14
	atHwcap  = 16
	atClktck = 17
	atSecure = 23
	atRandom = 25
	atHwcap2 = 26
	atExecfn = 31
)
