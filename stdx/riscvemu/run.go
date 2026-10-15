package riscvemu

import (
	"crypto/rand"
	"encoding/binary"
	"os"
)

func readFile(path string) ([]byte, error) { return os.ReadFile(path) }

// stackTop is where the initial user stack starts (grows down).
const stackTop = 0x0000ffffffff0000
const stackSize = 8 << 20 // 8 MiB

// Options configures how a binary is run.
type Options struct {
	Args []string // program args (without argv[0]); empty means just argv[0]
	Env  []string
}

// RunELF runs a riscv64 ELF binary and returns its exit code.
func RunELF(data []byte, opts Options) (int, error) {
	prog, mem, entry, err := LoadELF(data)
	if err != nil {
		return -1, err
	}
	machine := NewMachine(prog, mem)
	if os.Getenv("RISCVEMU_TRACE") != "" {
		machine.trace = true
	}
	if os.Getenv("RISCVEMU_TRACESYS") != "" {
		machine.traceSys = true
	}
	if os.Getenv("RISCVEMU_TRACESP") != "" {
		machine.traceSp = true
	}
	machine.minSp = ^uint64(0)

	// build initial stack with argc/argv/envp/auxv
	sp, auxv := buildStack(mem, opts)
	machine.auxvBytes = encodeAuxv(auxv)
	machine.stackStart = sp
	machine.mmapCursor = 0x0000004000000000

	t := machine.newThread(prog.addrsToSlot(entry))
	t.x[2] = sp // sp

	return machine.Schedule()
}

// RunELFFile runs a riscv64 ELF file from disk.
func RunELFFile(path string, opts Options) (int, error) {
	data, err := readFile(path)
	if err != nil {
		return -1, err
	}
	return RunELF(data, opts)
}

// buildStack writes the initial process stack and returns the initial sp
// plus the auxv vector (tag,value pairs).
func buildStack(mem *Mem, opts Options) (uint64, []uint64) {
	// We build the stack top-down: push strings first, then pointers.
	// Simple approach: build in a temporary buffer starting at stackTop,
	// growing downward.
	type word struct{ addr uint64 }

	var strings []string
	strings = append(strings, opts.Args...) // argv strings
	env := opts.Env

	sp := uint64(stackTop)

	// align down to 16
	sp &^= 15

	// We'll write from high to low. Strategy:
	//   [0]     argc
	//   [1..]   argv pointers, NULL
	//   [..]    envp pointers, NULL
	//   [..]    auxv pairs, AT_NULL
	//   [..]    string data (argv strings, env strings, path)
	// sp points at argc.

	// Place strings first at the very top of the stack area.
	strBase := uint64(stackTop) - 16
	var strAddrs []uint64
	for _, s := range append(append([]string{}, strings...), env...) {
		// write s + NUL
		sp2 := strBase - uint64(len(s)) - 1
		sp2 &^= 7 // align
		b := []byte(s)
		for i, c := range b {
			mem.WriteU8(sp2+uint64(i), c)
		}
		mem.WriteU8(sp2+uint64(len(s)), 0)
		strAddrs = append(strAddrs, sp2)
		strBase = sp2 - 8
	}

	// Now build the pointer area below strBase.
	p := strBase
	argc := len(opts.Args)
	nWords := 1 + argc + 1 + len(env) + 1 + 2*(len(auxvTable(len(env))))
	p -= uint64(nWords) * 8
	p &^= 15

	off := p
	w := func(v uint64) {
		mem.WriteU64(off, v)
		off += 8
	}
	w(uint64(argc))
	for i := 0; i < argc; i++ {
		w(strAddrs[i])
	}
	w(0) // argv NULL
	for i := 0; i < len(env); i++ {
		w(strAddrs[argc+i])
	}
	w(0) // envp NULL

	auxv := makeAuxv(len(env))
	for _, pair := range auxv {
		w(pair[0])
		w(pair[1])
	}
	w(atNull)
	w(0)

	return p, flattenAuxv(auxv)
}

func auxvTable(nenv int) [][2]uint64 { return makeAuxv(nenv) }

func makeAuxv(nenv int) [][2]uint64 {
	// 16 random bytes for AT_RANDOM
	var rnd [16]byte
	rand.Read(rnd[:])

	auxv := [][2]uint64{
		{atPagesz, PageSize},
		{atSecure, 0},
		{atUid, 0},
		{atEuid, 0},
		{atGid, 0},
		{atEgid, 0},
		{atHwcap, 0},
		{atHwcap2, 0},
		{atClktck, 100},
		{atPhdr, 0}, // filled by caller if needed
		{atEntry, 0},
	}
	// AT_RANDOM points into the stack; we store the value of rnd embedded in
	// the auxv itself via a reserved slot filled in buildStack.
	_ = nenv
	return auxv
}

func flattenAuxv(pairs [][2]uint64) []uint64 {
	var v []uint64
	for _, p := range pairs {
		v = append(v, p[0], p[1])
	}
	v = append(v, atNull, 0)
	return v
}

func encodeAuxv(pairs []uint64) []byte {
	b := make([]byte, len(pairs)*8)
	for i, v := range pairs {
		binary.LittleEndian.PutUint64(b[i*8:], v)
	}
	return b
}
