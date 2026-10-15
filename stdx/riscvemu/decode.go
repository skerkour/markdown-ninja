package riscvemu

import (
	"math"
	"math/bits"
)

// A closure is one compiled instruction. It returns the number of 2-byte
// "slots" to advance the instruction pointer (1 for a 16-bit instruction,
// 2 for a 32-bit instruction). Control transfer returns the offset to the
// target slot relative to the current slot.
//
// This implements the VM design from the PlanetScale article: the program is
// a slice of closures, each pre-specialized with its decoded operands, and
// the VM loop is simply
//
//	ip += code[ip](vm)
type closure func(*VM) int

// program is a compiled text segment.
type program struct {
	code []closure
	base uint64 // vaddr of slot 0
	size uint64 // text size in bytes

	// when recordRejects is set, fault() records the faulting address
	recordRejects bool
	rejected      []uint64
	currentSlot   int
}

func (p *program) addr(s int) uint64 { return p.base + uint64(s)*2 }

func (p *program) addrsToSlot(a uint64) int {
	if a < p.base || a >= p.base+p.size || a&1 != 0 {
		return -1
	}
	return int((a - p.base) >> 1)
}

const (
	trapNone = iota
	trapEcall
	trapEbreak
)

// compile builds a closure program from a contiguous text segment.
func compile(seg []byte, base uint64) *program {
	n := len(seg) / 2
	code := make([]closure, n)
	p := &program{code: code, base: base, size: uint64(len(seg))}

	for off := 0; off+2 <= len(seg); {
		slot := off / 2
		p.currentSlot = slot
		lo := uint16(seg[off]) | uint16(seg[off+1])<<8
		if lo&0x3 == 0x3 {
			if off+4 > len(seg) {
				code[slot] = p.fault("truncated instruction")
				break
			}
			w := uint32(lo) | uint32(seg[off+2])<<16 | uint32(seg[off+3])<<24
			code[slot] = p.decode32(slot, w)
			if slot+1 < n {
				code[slot+1] = p.fault("executed middle of 32-bit instruction")
			}
			off += 4
		} else {
			code[slot] = p.decode16(slot, lo)
			off += 2
		}
	}
	return p
}

func (p *program) fault(msg string) closure {
	if p.recordRejects {
		p.rejected = append(p.rejected, p.base+uint64(p.currentSlot)*2)
	}
	return func(vm *VM) int {
		vm.fault(msg)
		return 1
	}
}

func (p *program) notimpl(name string, w uint32) closure {
	return p.fault("unsupported instruction " + name)
}

func bool64(b bool, one uint64) uint64 {
	if b {
		return one
	}
	return 0
}

// to32 sign-extends a 32-bit value.
func to32(v uint64) uint64 { return uint64(int32(v)) }

// --- immediates -------------------------------------------------------------

func imm12(v uint32) int64 { return int64(int32(v)>>20) >> 12 << 12 >> 12 } // unused, see below

func jalImm(w uint32) int64 {
	i := (w>>31&1)<<20 | (w>>12&0xff)<<12 | (w>>20&1)<<11 | (w>>21&0x3ff)<<1
	return int64(int32(i<<11) >> 11)
}

func branchImm(w uint32) int64 {
	i := (w>>31&1)<<12 | (w>>7&1)<<11 | (w>>25&0x3f)<<5 | (w>>8&0xf)<<1
	return int64(int32(i<<19) >> 19)
}

func sImm(w uint32) int64 {
	i := ((w >> 25) & 0x7f) << 5
	i |= (w >> 7) & 0x1f
	return int64(int32(i<<20) >> 20)
}

func iImm(w uint32) int64 {
	return int64(int32(w)) >> 20 // sign-extend 12 bits: int32(w) already includes imm at [31:20]
}

// --- load/store -------------------------------------------------------------

func (p *program) load(f3 uint32, rd, rs1 int, imm uint64) closure {
	a := func(vm *VM) uint64 { return vm.x[rs1] + imm }
	switch f3 {
	case 0:
		return func(vm *VM) int {
			vm.reg(rd, uint64(int64(int8(vm.mem.ReadU8(a(vm))))))
			return 2
		}
	case 1:
		return func(vm *VM) int {
			vm.reg(rd, uint64(int64(int16(vm.mem.ReadU16(a(vm))))))
			return 2
		}
	case 2:
		return func(vm *VM) int {
			vm.reg(rd, uint64(int64(int32(vm.mem.ReadU32(a(vm))))))
			return 2
		}
	case 3:
		return func(vm *VM) int {
			vm.reg(rd, vm.mem.ReadU64(a(vm)))
			return 2
		}
	case 4:
		return func(vm *VM) int {
			vm.reg(rd, uint64(vm.mem.ReadU8(a(vm))))
			return 2
		}
	case 5:
		return func(vm *VM) int {
			vm.reg(rd, uint64(vm.mem.ReadU16(a(vm))))
			return 2
		}
	case 6:
		return func(vm *VM) int {
			vm.reg(rd, uint64(vm.mem.ReadU32(a(vm))))
			return 2
		}
	}
	return p.fault("bad load")
}

// --- decode32 ---------------------------------------------------------------

func (p *program) decode32(slot int, w uint32) closure {
	op := w & 0x7f
	f3 := (w >> 12) & 7
	rd := int((w >> 7) & 0x1f)
	rs1 := int((w >> 15) & 0x1f)
	rs2 := int((w >> 20) & 0x1f)
	f7 := (w >> 25) & 0x7f
	addr := p.base + uint64(slot)*2
	self := slot

	switch op {
	case 0x0f: // FENCE / FENCE.I
		return func(vm *VM) int { return 2 }
	case 0x37: // LUI
		v := uint64(int64(int32(w & 0xfffff000)))
		return func(vm *VM) int {
			vm.reg(rd, v)
			return 2
		}
	case 0x17: // AUIPC
		v := uint64(int64(int32(w & 0xfffff000)))
		pc := addr
		return func(vm *VM) int {
			vm.reg(rd, pc+v)
			return 2
		}
	case 0x6f: // JAL
		imm := jalImm(w)
		tgt := p.addrsToSlot(uint64(int64(addr) + imm))
		if tgt < 0 {
			return p.fault("jal target outside program")
		}
		d := tgt - self
		rdv := uint64(addr + 4)
		return func(vm *VM) int {
			vm.reg(rd, rdv)
			return d
		}
	case 0x67: // JALR
		if f3 != 0 {
			return p.fault("bad jalr")
		}
		imm := int64(int32(w) >> 20)
		rdv := uint64(addr + 4)
		a := rs1
		return func(vm *VM) int {
			t := (int64(vm.x[a]) + imm) &^ 1
			if uint64(t) == sigreturnSentinel {
				vm.doSigReturn()
				return 0
			}
			ts := p.addrsToSlot(uint64(t))
			if ts < 0 {
				vm.fault("jalr target outside program")
				return 1
			}
			vm.reg(rd, rdv)
			return ts - self
		}
	case 0x63: // BRANCH
		imm := branchImm(w)
		tgt := p.addrsToSlot(uint64(int64(addr) + imm))
		if tgt < 0 {
			return p.fault("branch target outside program")
		}
		d := tgt - self
		switch f3 {
		case 0:
			return func(vm *VM) int {
				if vm.x[rs1] == vm.x[rs2] {
					return d
				}
				return 2
			}
		case 1:
			return func(vm *VM) int {
				if vm.x[rs1] != vm.x[rs2] {
					return d
				}
				return 2
			}
		case 4:
			return func(vm *VM) int {
				if int64(vm.x[rs1]) < int64(vm.x[rs2]) {
					return d
				}
				return 2
			}
		case 5:
			return func(vm *VM) int {
				if int64(vm.x[rs1]) >= int64(vm.x[rs2]) {
					return d
				}
				return 2
			}
		case 6:
			return func(vm *VM) int {
				if vm.x[rs1] < vm.x[rs2] {
					return d
				}
				return 2
			}
		case 7:
			return func(vm *VM) int {
				if vm.x[rs1] >= vm.x[rs2] {
					return d
				}
				return 2
			}
		}
		return p.fault("bad branch")
	case 0x03: // LOAD
		imm := iImm(w)
		return p.load(f3, rd, rs1, uint64(imm))
	case 0x07: // LOAD-FP
		imm := iImm(w)
		switch f3 {
		case 2: // FLW
			return func(vm *VM) int {
				bits := uint64(vm.mem.ReadU32(uint64(int64(vm.x[rs1]) + imm)))
				vm.f[rd] = bits | 0xffffffff00000000
				return 2
			}
		case 3: // FLD
			return func(vm *VM) int {
				vm.f[rd] = vm.mem.ReadU64(uint64(int64(vm.x[rs1]) + imm))
				return 2
			}
		}
		return p.fault("bad load-fp")
	case 0x23: // STORE
		imm := sImm(w)
		switch f3 {
		case 0:
			return func(vm *VM) int {
				vm.mem.WriteU8(uint64(int64(vm.x[rs1])+imm), byte(vm.x[rs2]))
				return 2
			}
		case 1:
			return func(vm *VM) int {
				vm.mem.WriteU16(uint64(int64(vm.x[rs1])+imm), uint16(vm.x[rs2]))
				return 2
			}
		case 2:
			return func(vm *VM) int {
				vm.mem.WriteU32(uint64(int64(vm.x[rs1])+imm), uint32(vm.x[rs2]))
				return 2
			}
		case 3:
			return func(vm *VM) int {
				vm.mem.WriteU64(uint64(int64(vm.x[rs1])+imm), vm.x[rs2])
				return 2
			}
		}
		return p.fault("bad store")
	case 0x27: // STORE-FP
		imm := sImm(w)
		switch f3 {
		case 2:
			return func(vm *VM) int {
				vm.mem.WriteU32(uint64(int64(vm.x[rs1])+imm), uint32(vm.f[rs2]))
				return 2
			}
		case 3:
			return func(vm *VM) int {
				vm.mem.WriteU64(uint64(int64(vm.x[rs1])+imm), vm.f[rs2])
				return 2
			}
		}
		return p.fault("bad store-fp")
	case 0x13: // OP-IMM
		imm := iImm(w)
		switch f3 {
		case 0:
			return func(vm *VM) int {
				vm.reg(rd, uint64(int64(vm.x[rs1])+imm))
				return 2
			}
		case 1: // SLLI
			sh := uint64((w >> 20) & 0x3f)
			return func(vm *VM) int {
				vm.reg(rd, vm.x[rs1]<<sh)
				return 2
			}
		case 2:
			return func(vm *VM) int {
				vm.reg(rd, bool64(int64(vm.x[rs1]) < imm, 1))
				return 2
			}
		case 3:
			return func(vm *VM) int {
				vm.reg(rd, bool64(vm.x[rs1] < uint64(imm), 1))
				return 2
			}
		case 4:
			return func(vm *VM) int {
				vm.reg(rd, vm.x[rs1]^uint64(imm))
				return 2
			}
		case 5: // SRLI / SRAI (funct6 bits 31:26: 0x00=SRLI, 0x10=SRAI; shamt includes bit 25)
			sh := uint64((w >> 20) & 0x3f)
			if (w>>26)&0x3f == 0 {
				return func(vm *VM) int {
					vm.reg(rd, vm.x[rs1]>>sh)
					return 2
				}
			}
			return func(vm *VM) int {
				vm.reg(rd, uint64(int64(vm.x[rs1])>>sh))
				return 2
			}
		case 6:
			return func(vm *VM) int {
				vm.reg(rd, vm.x[rs1]|uint64(imm))
				return 2
			}
		case 7:
			return func(vm *VM) int {
				vm.reg(rd, vm.x[rs1]&uint64(imm))
				return 2
			}
		}
		return p.fault("bad op-imm")
	case 0x1b: // OP-IMM-32
		imm := iImm(w)
		switch f3 {
		case 0: // ADDIW
			return func(vm *VM) int {
				vm.reg(rd, to32(uint64(uint32(int32(vm.x[rs1])+int32(imm)))))
				return 2
			}
		case 1: // SLLIW
			sh := uint32((w >> 20) & 0x1f)
			return func(vm *VM) int {
				vm.reg(rd, to32(uint64(uint32(int32(vm.x[rs1])<<sh))))
				return 2
			}
		case 5: // SRLIW / SRAIW
			sh := uint32((w >> 20) & 0x1f)
			if f7 == 0 {
				return func(vm *VM) int {
					vm.reg(rd, to32(uint64(uint32(vm.x[rs1])>>sh)))
					return 2
				}
			}
			return func(vm *VM) int {
				vm.reg(rd, to32(uint64(uint32(int32(vm.x[rs1])>>sh))))
				return 2
			}
		}
		return p.fault("bad op-imm32")
	case 0x33: // OP
		if f7 == 0x01 {
			return p.mul(rd, rs1, rs2, int(f3))
		}
		switch f3 {
		case 0:
			if f7 == 0x20 {
				return func(vm *VM) int {
					vm.reg(rd, vm.x[rs1]-vm.x[rs2])
					return 2
				}
			}
			return func(vm *VM) int {
				vm.reg(rd, vm.x[rs1]+vm.x[rs2])
				return 2
			}
		case 1:
			return func(vm *VM) int {
				vm.reg(rd, vm.x[rs1]<<(vm.x[rs2]&0x3f))
				return 2
			}
		case 2:
			return func(vm *VM) int {
				vm.reg(rd, bool64(int64(vm.x[rs1]) < int64(vm.x[rs2]), 1))
				return 2
			}
		case 3:
			return func(vm *VM) int {
				vm.reg(rd, bool64(vm.x[rs1] < vm.x[rs2], 1))
				return 2
			}
		case 4:
			return func(vm *VM) int {
				vm.reg(rd, vm.x[rs1]^vm.x[rs2])
				return 2
			}
		case 5:
			if f7 == 0x20 {
				return func(vm *VM) int {
					vm.reg(rd, uint64(int64(vm.x[rs1])>>(vm.x[rs2]&0x3f)))
					return 2
				}
			}
			return func(vm *VM) int {
				vm.reg(rd, vm.x[rs1]>>(vm.x[rs2]&0x3f))
				return 2
			}
		case 6:
			return func(vm *VM) int {
				vm.reg(rd, vm.x[rs1]|vm.x[rs2])
				return 2
			}
		case 7:
			return func(vm *VM) int {
				vm.reg(rd, vm.x[rs1]&vm.x[rs2])
				return 2
			}
		}
		return p.fault("bad op")
	case 0x3b: // OP-32
		if f7 == 0x01 {
			return p.mulw(rd, rs1, rs2, int(f3))
		}
		switch f3 {
		case 0:
			if f7 == 0x20 {
				return func(vm *VM) int {
					vm.reg(rd, to32(vm.x[rs1]-vm.x[rs2]))
					return 2
				}
			}
			return func(vm *VM) int {
				vm.reg(rd, to32(vm.x[rs1]+vm.x[rs2]))
				return 2
			}
		case 1:
			return func(vm *VM) int {
				vm.reg(rd, to32(vm.x[rs1]<<(vm.x[rs2]&0x1f)))
				return 2
			}
		case 5:
			if f7 == 0x20 {
				return func(vm *VM) int {
					vm.reg(rd, to32(uint64(uint32(int32(vm.x[rs1])>>(vm.x[rs2]&0x1f)))))
					return 2
				}
			}
			return func(vm *VM) int {
				vm.reg(rd, to32(vm.x[rs1]>>(vm.x[rs2]&0x1f)))
				return 2
			}
		}
		return p.fault("bad op-32")
	case 0x2f: // AMO
		f5 := w >> 27
		aq := (w >> 26) & 1
		rl := (w >> 25) & 1
		_ = aq
		_ = rl
		switch f3 {
		case 2:
			return p.amo("w", uint64(f5), rd, rs1, rs2, w)
		case 3:
			return p.amo("d", uint64(f5), rd, rs1, rs2, w)
		}
		return p.fault("bad amo")
	case 0x73: // SYSTEM
		f12 := (w >> 20) & 0xfff
		switch {
		case f12 == 0x000:
			return func(vm *VM) int {
				vm.trap = trapEcall
				return 2
			}
		case f12 == 0x001:
			return func(vm *VM) int {
				vm.trap = trapEbreak
				return 2
			}
		}
		csr := int(f12)
		switch f3 {
		case 1: // CSRRW
			return func(vm *VM) int {
				old := vm.csr(csr)
				vm.reg(rd, old)
				vm.setcsr(csr, vm.x[rs1])
				return 2
			}
		case 2: // CSRRS
			return func(vm *VM) int {
				old := vm.csr(csr)
				if rs1 != 0 {
					vm.setcsr(csr, old|vm.x[rs1])
				}
				vm.reg(rd, old)
				return 2
			}
		case 3: // CSRRC
			return func(vm *VM) int {
				old := vm.csr(csr)
				if rs1 != 0 {
					vm.setcsr(csr, old&^vm.x[rs1])
				}
				vm.reg(rd, old)
				return 2
			}
		case 5: // CSRRWI
			return func(vm *VM) int {
				old := vm.csr(csr)
				vm.reg(rd, old)
				vm.setcsr(csr, uint64(rs1))
				return 2
			}
		case 6: // CSRRSI
			return func(vm *VM) int {
				old := vm.csr(csr)
				if rs1 != 0 {
					vm.setcsr(csr, old|uint64(rs1))
				}
				vm.reg(rd, old)
				return 2
			}
		case 7: // CSRRCI
			return func(vm *VM) int {
				old := vm.csr(csr)
				if rs1 != 0 {
					vm.setcsr(csr, old&^uint64(rs1))
				}
				vm.reg(rd, old)
				return 2
			}
		}
		return p.fault("bad system")
	case 0x53: // FP
		return p.fpop(rd, rs1, rs2, uint(f3), uint(f7))
	case 0x43, 0x47, 0x4b, 0x4f: // FMA
		return p.fma(w)
	}
	return p.notimpl("unknown", w)
}

// --- M extension ------------------------------------------------------------

func (p *program) mul(rd, rs1, rs2, sub int) closure {
	switch sub {
	case 0x0: // MUL
		return func(vm *VM) int {
			vm.reg(rd, vm.x[rs1]*vm.x[rs2])
			return 2
		}
	case 0x1: // MULH
		return func(vm *VM) int {
			hi, _ := bits.Mul64(vm.x[rs1], vm.x[rs2])
			a, b := int64(vm.x[rs1]), int64(vm.x[rs2])
			if a < 0 {
				hi -= vm.x[rs2]
			}
			if b < 0 {
				hi -= vm.x[rs1]
			}
			vm.reg(rd, hi)
			return 2
		}
	case 0x2: // MULHSU
		return func(vm *VM) int {
			hi, _ := bits.Mul64(vm.x[rs1], vm.x[rs2])
			if int64(vm.x[rs1]) < 0 {
				hi -= vm.x[rs2]
			}
			vm.reg(rd, hi)
			return 2
		}
	case 0x3: // MULHU
		return func(vm *VM) int {
			hi, _ := bits.Mul64(vm.x[rs1], vm.x[rs2])
			vm.reg(rd, hi)
			return 2
		}
	case 0x4: // DIV
		return func(vm *VM) int {
			a, b := int64(vm.x[rs1]), int64(vm.x[rs2])
			var v int64
			switch {
			case b == 0:
				v = -1
			case a == math.MinInt64 && b == -1:
				v = math.MinInt64
			default:
				v = a / b
			}
			vm.reg(rd, uint64(v))
			return 2
		}
	case 0x5: // DIVU
		return func(vm *VM) int {
			a, b := vm.x[rs1], vm.x[rs2]
			var v uint64
			if b == 0 {
				v = math.MaxUint64
			} else {
				v = a / b
			}
			vm.reg(rd, v)
			return 2
		}
	case 0x6: // REM
		return func(vm *VM) int {
			a, b := int64(vm.x[rs1]), int64(vm.x[rs2])
			var v int64
			switch {
			case b == 0:
				v = a
			case a == math.MinInt64 && b == -1:
				v = 0
			default:
				v = a % b
			}
			vm.reg(rd, uint64(v))
			return 2
		}
	case 0x7: // REMU
		return func(vm *VM) int {
			a, b := vm.x[rs1], vm.x[rs2]
			var v uint64
			if b == 0 {
				v = a
			} else {
				v = a % b
			}
			vm.reg(rd, v)
			return 2
		}
	}
	return p.fault("bad m-op")
}

func (p *program) mulw(rd, rs1, rs2, sub int) closure {
	switch sub {
	case 0x0:
		return func(vm *VM) int {
			vm.reg(rd, to32(vm.x[rs1]*vm.x[rs2]))
			return 2
		}
	case 0x4: // DIVW
		return func(vm *VM) int {
			a, b := int32(vm.x[rs1]), int32(vm.x[rs2])
			var v int32
			switch {
			case b == 0:
				v = -1
			case a == math.MinInt32 && b == -1:
				v = math.MinInt32
			default:
				v = a / b
			}
			vm.reg(rd, to32(uint64(uint32(v))))
			return 2
		}
	case 0x5: // DIVUW
		return func(vm *VM) int {
			a, b := uint32(vm.x[rs1]), uint32(vm.x[rs2])
			var v uint32
			if b == 0 {
				v = math.MaxUint32
			} else {
				v = a / b
			}
			vm.reg(rd, to32(uint64(v)))
			return 2
		}
	case 0x6: // REMW
		return func(vm *VM) int {
			a, b := int32(vm.x[rs1]), int32(vm.x[rs2])
			var v int32
			switch {
			case b == 0:
				v = a
			case a == math.MinInt32 && b == -1:
				v = 0
			default:
				v = a % b
			}
			vm.reg(rd, to32(uint64(uint32(v))))
			return 2
		}
	case 0x7: // REMUW
		return func(vm *VM) int {
			a, b := uint32(vm.x[rs1]), uint32(vm.x[rs2])
			var v uint32
			if b == 0 {
				v = a
			} else {
				v = a % b
			}
			vm.reg(rd, to32(uint64(v)))
			return 2
		}
	}
	return p.fault("bad m-w-op")
}

// --- AMO --------------------------------------------------------------------

func (p *program) amo(width string, f5 uint64, rd, rs1, rs2 int, w uint32) closure {
	switch f5 {
	case 0x02: // LR
		if width == "w" {
			return func(vm *VM) int {
				vm.reg(rd, to32(uint64(vm.mem.ReadU32(vm.x[rs1]))))
				return 2
			}
		}
		return func(vm *VM) int {
			vm.reg(rd, vm.mem.ReadU64(vm.x[rs1]))
			return 2
		}
	case 0x03: // SC
		if width == "w" {
			return func(vm *VM) int {
				vm.mem.WriteU32(vm.x[rs1], uint32(vm.x[rs2]))
				vm.reg(rd, 0)
				return 2
			}
		}
		return func(vm *VM) int {
			vm.mem.WriteU64(vm.x[rs1], vm.x[rs2])
			vm.reg(rd, 0)
			return 2
		}
	}
	mask := uint64(math.MaxUint64)
	if width == "w" {
		mask = math.MaxUint32
	}
	op := func(vm *VM) uint64 {
		a := vm.x[rs1]
		old := vm.mem.ReadU64(a)
		if width == "w" {
			old = to32(old)
		}
		var nv uint64
		switch f5 {
		case 0x01: // SWAP
			nv = vm.x[rs2]
		case 0x00: // ADD
			nv = old + vm.x[rs2]
		case 0x04: // XOR
			nv = old ^ vm.x[rs2]
		case 0x0c: // AND
			nv = old & vm.x[rs2]
		case 0x08: // OR
			nv = old | vm.x[rs2]
		case 0x10: // MIN
			if int64(vm.x[rs2]) < int64(old) {
				nv = vm.x[rs2]
			} else {
				nv = old
			}
		case 0x14: // MAX
			if int64(vm.x[rs2]) > int64(old) {
				nv = vm.x[rs2]
			} else {
				nv = old
			}
		case 0x18: // MINU
			if vm.x[rs2] < old {
				nv = vm.x[rs2]
			} else {
				nv = old
			}
		case 0x1c: // MAXU
			if vm.x[rs2] > old {
				nv = vm.x[rs2]
			} else {
				nv = old
			}
		default:
			vm.fault("bad amo op")
			return old
		}
		nv &= mask
		if width == "w" {
			vm.mem.WriteU32(a, uint32(nv))
		} else {
			vm.mem.WriteU64(a, nv)
		}
		return old & mask
	}
	return func(vm *VM) int {
		vm.reg(rd, op(vm))
		return 2
	}
}
