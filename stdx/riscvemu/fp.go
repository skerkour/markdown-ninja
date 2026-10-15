package riscvemu

import "math"

// fpop decodes OP-FP (0x53) instructions for the F and D extensions.
// f7 is the funct7 field, f3 the rounding mode field.
func (p *program) fpop(rd, rs1, rs2 int, f3, f7 uint) closure {
	if rd < 0 || rd > 31 || rs1 < 0 || rs1 > 31 || rs2 < 0 || rs2 > 31 {
		return p.fault("bad FP register")
	}
	isD := f7&0x01 == 0x01

	read := func(vm *VM, r int) float64 {
		if isD {
			return math.Float64frombits(vm.f[r])
		}
		return float64(math.Float32frombits(uint32(vm.f[r])))
	}
	write := func(vm *VM, r int, v float64) {
		if isD {
			vm.f[r] = math.Float64bits(v)
		} else {
			vm.f[r] = uint64(math.Float32bits(float32(v))) | 0xffffffff00000000
		}
	}

	switch {
	case f7 == 0x00 || f7 == 0x01: // FADD
		return func(vm *VM) int {
			write(vm, rd, read(vm, rs1)+read(vm, rs2))
			return 2
		}
	case f7 == 0x04 || f7 == 0x05: // FSUB
		return func(vm *VM) int {
			write(vm, rd, read(vm, rs1)-read(vm, rs2))
			return 2
		}
	case f7 == 0x08 || f7 == 0x09: // FMUL
		return func(vm *VM) int {
			write(vm, rd, read(vm, rs1)*read(vm, rs2))
			return 2
		}
	case f7 == 0x0c || f7 == 0x0d: // FDIV
		return func(vm *VM) int {
			write(vm, rd, read(vm, rs1)/read(vm, rs2))
			return 2
		}
	case f7 == 0x10 || f7 == 0x11: // FSGNJ / FSGNJN / FSGNJX
		switch f3 {
		case 0:
			return func(vm *VM) int {
				bits := (vm.f[rs1] &^ (1 << 63)) | (vm.f[rs2] & (1 << 63))
				vm.f[rd] = bits
				return 2
			}
		case 1:
			return func(vm *VM) int {
				bits := (vm.f[rs1] &^ (1 << 63)) | (^vm.f[rs2] & (1 << 63))
				vm.f[rd] = bits
				return 2
			}
		case 2:
			return func(vm *VM) int {
				bits := vm.f[rs1] ^ (vm.f[rs2] & (1 << 63))
				vm.f[rd] = bits
				return 2
			}
		}
		return p.fault("bad fsgnj")
	case f7 == 0x14 || f7 == 0x15: // FMIN / FMAX
		min := f3 == 0
		return func(vm *VM) int {
			a, b := read(vm, rs1), read(vm, rs2)
			if math.IsNaN(a) {
				write(vm, rd, b)
			} else if math.IsNaN(b) {
				write(vm, rd, a)
			} else if min {
				if a < b {
					write(vm, rd, a)
				} else {
					write(vm, rd, b)
				}
			} else {
				if a > b {
					write(vm, rd, a)
				} else {
					write(vm, rd, b)
				}
			}
			return 2
		}
	case f7 == 0x20: // FCVT.S.D
		return func(vm *VM) int {
			v := math.Float32frombits(uint32(vm.f[rs1]))
			vm.f[rd] = uint64(math.Float32bits(v)) | 0xffffffff00000000
			return 2
		}
	case f7 == 0x21: // FCVT.D.S
		return func(vm *VM) int {
			v := float64(math.Float32frombits(uint32(vm.f[rs1])))
			vm.f[rd] = math.Float64bits(v)
			return 2
		}
	case f7 == 0x2c: // FSQRT
		return func(vm *VM) int {
			write(vm, rd, math.Sqrt(read(vm, rs1)))
			return 2
		}
	case f7 == 0x2d: // FSQRT.D is under 0x2d; handle both
		return func(vm *VM) int {
			write(vm, rd, math.Sqrt(read(vm, rs1)))
			return 2
		}
	case f7 == 0x50 || f7 == 0x51: // FLE / FLT / FEQ
		return func(vm *VM) int {
			a, b := read(vm, rs1), read(vm, rs2)
			var c bool
			switch f3 {
			case 0:
				c = a <= b
			case 1:
				c = a < b
			case 2:
				c = a == b
			}
			vm.reg(rd, bool64(c, 1))
			return 2
		}
	case f7 == 0x60 || f7 == 0x61: // FCVT.W.S / FCVT.WU.S / FCVT.L.S / FCVT.LU.S
		conv := int(f3)
		return func(vm *VM) int {
			v := read(vm, rs1)
			vm.reg(rd, fcvtInt(conv, v))
			return 2
		}
	case f7 == 0x70 || f7 == 0x71: // FMV.X.W / FCLASS; and FMV.X.D
		switch f3 {
		case 0: // FMV.X.W / FMV.X.D
			return func(vm *VM) int {
				if isD {
					vm.reg(rd, vm.f[rs1])
				} else {
					vm.reg(rd, to32(vm.f[rs1]))
				}
				return 2
			}
		case 1: // FCLASS
			return func(vm *VM) int {
				vm.reg(rd, fclass(read(vm, rs1)))
				return 2
			}
		}
		return p.fault("bad fmv/fclass")
	case f7 == 0x68 || f7 == 0x69: // FCVT.S.W / FCVT.S.WU / FCVT.S.L / FCVT.S.LU
		conv := int(f3)
		return func(vm *VM) int {
			write(vm, rd, fcvtFromInt(conv, vm.x[rs1]))
			return 2
		}
	case f7 == 0x78 || f7 == 0x79: // FMV.W.X / FMV.D.X
		return func(vm *VM) int {
			if isD {
				vm.f[rd] = vm.x[rs1]
			} else {
				vm.f[rd] = vm.x[rs1] | 0xffffffff00000000
			}
			return 2
		}
	}
	return p.fault("unhandled FP op")
}

// fma decodes FMADD/FMSUB/FNMSUB/FNMADD (opcodes 0x43/0x47/0x4b/0x4f).
// Encoding: funct7(7) fmt(2) rs3(5) rs1(5) rm(3) rd(5) rs2(5) opcode(5).
func (p *program) fma(w uint32) closure {
	op := w & 0x7f
	fmt := (w >> 25) & 0x3
	rs3 := int((w >> 20) & 0x1f)
	rd := int((w >> 7) & 0x1f)
	rs1 := int((w >> 15) & 0x1f)
	rs2 := int((w >> 2) & 0x1f)
	isD := fmt == 0x1

	read := func(vm *VM, r int) float64 {
		if isD {
			return math.Float64frombits(vm.f[r])
		}
		return float64(math.Float32frombits(uint32(vm.f[r])))
	}
	write := func(vm *VM, r int, v float64) {
		if isD {
			vm.f[r] = math.Float64bits(v)
		} else {
			vm.f[r] = uint64(math.Float32bits(float32(v))) | 0xffffffff00000000
		}
	}
	switch op {
	case 0x43: // FMADD: rd = rs1*rs2+rs3
		return func(vm *VM) int {
			write(vm, rd, read(vm, rs1)*read(vm, rs2)+read(vm, rs3))
			return 2
		}
	case 0x47: // FMSUB: rd = rs1*rs2-rs3
		return func(vm *VM) int {
			write(vm, rd, read(vm, rs1)*read(vm, rs2)-read(vm, rs3))
			return 2
		}
	case 0x4b: // FNMSUB: rd = -rs1*rs2+rs3
		return func(vm *VM) int {
			write(vm, rd, -read(vm, rs1)*read(vm, rs2)+read(vm, rs3))
			return 2
		}
	case 0x4f: // FNMADD: rd = -rs1*rs2-rs3
		return func(vm *VM) int {
			write(vm, rd, -read(vm, rs1)*read(vm, rs2)-read(vm, rs3))
			return 2
		}
	}
	return p.fault("bad fma")
}

// fcvtInt implements FCVT.W.S / FCVT.L.S etc. conv: 0=W signed,1=WU,2=L signed,3=LU.
func fcvtInt(conv int, v float64) uint64 {
	switch conv {
	case 0:
		return to32(uint64(int64(v)))
	case 1:
		return uint64(uint32(v))
	case 2:
		return uint64(int64(v))
	case 3:
		return uint64(v)
	}
	return 0
}

func fcvtFromInt(conv int, v uint64) float64 {
	switch conv {
	case 0:
		return float64(int32(v))
	case 1:
		return float64(uint32(v))
	case 2:
		return float64(int64(v))
	case 3:
		return float64(v)
	}
	return 0
}

// fclass returns the RISC-V fclass mask for a float.
func fclass(v float64) uint64 {
	switch {
	case v != v:
		if math.Signbit(v) {
			return 1 << 1
		}
		return 1 << 0
	case v == 0:
		if math.Signbit(v) {
			return 1 << 3
		}
		return 1 << 4
	case math.IsInf(v, -1):
		return 1 << 2
	case math.IsInf(v, 1):
		return 1 << 5
	case v < 0:
		return 1 << 6
	default:
		return 1 << 7
	}
}
