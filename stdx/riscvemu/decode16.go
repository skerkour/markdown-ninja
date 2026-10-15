package riscvemu

// decode16 decodes a 16-bit RVC instruction at the given slot.
func (p *program) decode16(slot int, h uint16) closure {
	f3 := (h >> 13) & 7
	op := h & 3
	self := slot
	addr := p.base + uint64(slot)*2

	reg8 := func(x uint16) int { return int((h>>x)&0x7) + 8 } // rd'/rs1' are x8-x15
	rd := int(h >> 7 & 0x1f)
	rs1 := int(h >> 7 & 0x1f)
	rs2 := int(h >> 2 & 0x1f)

	switch op {
	case 0: // Quadrant 0
		switch f3 {
		case 0: // C.ADDI4SPN
			// nzuimm[5:4]=inst[12:11], nzuimm[9:6]=inst[10:7],
			// nzuimm[2]=inst[6], nzuimm[3]=inst[5]
			nzu := (uint32(h>>11)&0x3)<<4 | (uint32(h>>7)&0xf)<<6 | (uint32(h>>6)&0x1)<<2 | (uint32(h>>5)&0x1)<<3
			rd8 := reg8(2)
			if nzu == 0 {
				return p.fault("C.ADDI4SPN zero imm")
			}
			return func(vm *VM) int {
				vm.reg(rd8, vm.x[2]+uint64(nzu))
				return 1
			}
		case 1: // C.FLD (imm = uimm[5:3]<<3 | uimm[7:6]<<6)
			imm := uint32(h>>10&0x7)<<3 | uint32(h>>6&0x1)<<7 | uint32(h>>5&0x1)<<6
			r1 := reg8(7)
			rd8 := reg8(2)
			return func(vm *VM) int {
				vm.f[rd8] = vm.mem.ReadU64(vm.x[r1] + uint64(imm))
				return 1
			}
		case 2: // C.LW (imm = uimm[5:3]<<3 | uimm[2]<<2 | uimm[6]<<6)
			imm := uint32(h>>10&0x7)<<3 | uint32(h>>6&0x1)<<2 | uint32(h>>5&0x1)<<6
			r1 := reg8(7)
			rd8 := reg8(2)
			return func(vm *VM) int {
				vm.reg(rd8, uint64(int64(int32(vm.mem.ReadU32(vm.x[r1]+uint64(imm))))))
				return 1
			}
		case 3: // C.LD (imm = uimm[5:3]<<3 | uimm[7:6]<<6)
			imm := uint32(h>>10&0x7)<<3 | uint32(h>>6&0x1)<<7 | uint32(h>>5&0x1)<<6
			r1 := reg8(7)
			rd8 := reg8(2)
			return func(vm *VM) int {
				vm.reg(rd8, vm.mem.ReadU64(vm.x[r1]+uint64(imm)))
				return 1
			}
		case 5: // C.FSD (imm = uimm[5:3]<<3 | uimm[7:6]<<6)
			imm := uint32(h>>10&0x7)<<3 | uint32(h>>6&0x1)<<7 | uint32(h>>5&0x1)<<6
			r1 := reg8(7)
			rs8 := reg8(2)
			return func(vm *VM) int {
				vm.mem.WriteU64(vm.x[r1]+uint64(imm), vm.f[rs8])
				return 1
			}
		case 6: // C.SW (imm = uimm[5:3]<<3 | uimm[2]<<2 | uimm[6]<<6)
			imm := uint32(h>>10&0x7)<<3 | uint32(h>>6&0x1)<<2 | uint32(h>>5&0x1)<<6
			r1 := reg8(7)
			rs8 := reg8(2)
			return func(vm *VM) int {
				vm.mem.WriteU32(vm.x[r1]+uint64(imm), uint32(vm.x[rs8]))
				return 1
			}
		case 7: // C.SD (imm = uimm[5:3]<<3 | uimm[7:6]<<6)
			imm := uint32(h>>10&0x7)<<3 | uint32(h>>6&0x1)<<7 | uint32(h>>5&0x1)<<6
			r1 := reg8(7)
			rs8 := reg8(2)
			return func(vm *VM) int {
				vm.mem.WriteU64(vm.x[r1]+uint64(imm), vm.x[rs8])
				return 1
			}
		}
		return p.fault("C reserved h=" + sprintf("0x%x q=%d f3=%d", h, op, f3))

	case 1: // Quadrant 1
		switch f3 {
		case 0: // C.ADDI / C.NOP
			imm := int64(h>>12&1)<<5 | int64(h>>2&0x1f)
			imm = sext5(int64(h>>2&0x1f) | int64(h>>12&1)<<5)
			return func(vm *VM) int {
				vm.reg(rd, uint64(int64(vm.x[rd])+imm))
				return 1
			}
		case 1: // C.ADDIW (RV64; C.JAL in RV32)
			imm := sext5(int64(h>>2&0x1f) | int64(h>>12&1)<<5)
			return func(vm *VM) int {
				vm.reg(rd, to32(uint64(uint32(int32(vm.x[rd])+int32(imm)))))
				return 1
			}
		case 2: // C.LI
			imm := sext5(int64(h>>2&0x1f) | int64(h>>12&1)<<5)
			return func(vm *VM) int {
				vm.reg(rd, uint64(imm))
				return 1
			}
		case 3: // C.ADDI16SP / C.LUI
			if rd == 2 {
				imm := int64(h>>12&1)<<9 | int64(h>>6&1)<<4 | int64(h>>5&1)<<6 | int64(h>>3&0x3)<<7 | int64(h>>2&1)<<5
				imm = sext9(int64(h>>12&1)<<9 | int64(h>>6&1)<<4 | int64(h>>5&1)<<6 | int64(h>>3&0x3)<<7 | int64(h>>2&1)<<5)
				return func(vm *VM) int {
					vm.reg(2, uint64(int64(vm.x[2])+imm))
					return 1
				}
			}
			// C.LUI: nzimm[17:12] = {bit12, bits[6:2]} as a 6-bit value,
			// sign-extended, then shifted left 12.
			nz := int64(h>>12&1)<<5 | int64(h>>2&0x1f)
			if rd == 0 || nz == 0 {
				return p.fault("C.LUI")
			}
			imm := (nz << 58) >> 58 << 12
			return func(vm *VM) int {
				vm.reg(rd, uint64(imm))
				return 1
			}
		case 4: // funct3=100: SRLI/SRAI/ANDI/SUB/XOR/OR/AND/SUBW/ADDW
			rs1 = reg8(7)
			f2 := (h >> 10) & 3
			bit12 := (h >> 12) & 1
			switch {
			case f2 == 0: // C.SRLI
				sh := uint64(bit12)<<5 | uint64(h>>2&0x1f)
				return func(vm *VM) int {
					vm.reg(rs1, vm.x[rs1]>>sh)
					return 1
				}
			case f2 == 1: // C.SRAI
				sh := uint64(bit12)<<5 | uint64(h>>2&0x1f)
				return func(vm *VM) int {
					vm.reg(rs1, uint64(int64(vm.x[rs1])>>sh))
					return 1
				}
			case f2 == 2: // C.ANDI
				imm := sext5(int64(h>>2&0x1f) | int64(bit12)<<5)
				return func(vm *VM) int {
					vm.reg(rs1, vm.x[rs1]&uint64(imm))
					return 1
				}
			default: // f2 == 3: SUB/XOR/OR/AND (bit12=0) or SUBW/ADDW (bit12=1)
				rs2 := reg8(2)
				subop := (h >> 5) & 3
				if bit12 == 0 {
					switch subop {
					case 0:
						return func(vm *VM) int {
							vm.reg(rs1, vm.x[rs1]-vm.x[rs2])
							return 1
						}
					case 1:
						return func(vm *VM) int {
							vm.reg(rs1, vm.x[rs1]^vm.x[rs2])
							return 1
						}
					case 2:
						return func(vm *VM) int {
							vm.reg(rs1, vm.x[rs1]|vm.x[rs2])
							return 1
						}
					default:
						return func(vm *VM) int {
							vm.reg(rs1, vm.x[rs1]&vm.x[rs2])
							return 1
						}
					}
				}
				if subop == 0 { // C.SUBW
					return func(vm *VM) int {
						vm.reg(rs1, to32(vm.x[rs1]-vm.x[rs2]))
						return 1
					}
				}
				return func(vm *VM) int { // C.ADDW
					vm.reg(rs1, to32(vm.x[rs1]+vm.x[rs2]))
					return 1
				}
			}
		case 6, 7: // C.BEQZ / C.BNEZ
			// offset = sext9({imm[8],imm[7],imm[6],imm[5],imm[4],imm[3],imm[2],imm[1]})<<0
			// imm[8]=b12, imm[7]=b6, imm[6]=b5, imm[5]=b2, imm[4]=b11, imm[3]=b10,
			// imm[2]=b4, imm[1]=b3
			imm := int64(h>>12&1)<<8 | int64(h>>6&1)<<7 | int64(h>>5&1)<<6 | int64(h>>2&1)<<5 |
				int64(h>>11&1)<<4 | int64(h>>10&1)<<3 | int64(h>>4&1)<<2 | int64(h>>3&1)<<1
			imm = (imm << 55) >> 55 // sign extend 9 bits
			tgt := p.addrsToSlot(uint64(int64(addr) + imm))
			if tgt < 0 {
				return p.fault("C.BEQZ/BNEZ target")
			}
			d := tgt - self
			rs1 = reg8(7)
			if f3 == 6 {
				return func(vm *VM) int {
					if vm.x[rs1] == 0 {
						return d
					}
					return 1
				}
			}
			return func(vm *VM) int {
				if vm.x[rs1] != 0 {
					return d
				}
				return 1
			}
		case 5: // C.J
			// offset (byte) = sext12({b12, b8, b10, b9, b6, b7, b2, b11, b5, b4, b3})<<1
			// i.e. imm[11]=b12, imm[10]=b8, imm[9]=b10, imm[8]=b9, imm[7]=b6,
			//      imm[6]=b7, imm[5]=b2, imm[4]=b11, imm[3]=b5, imm[2]=b4, imm[1]=b3
			imm := int64(h>>12&1)<<11 | int64(h>>8&1)<<10 | int64(h>>10&1)<<9 | int64(h>>9&1)<<8 |
				int64(h>>6&1)<<7 | int64(h>>7&1)<<6 | int64(h>>2&1)<<5 | int64(h>>11&1)<<4 |
				int64(h>>5&1)<<3 | int64(h>>4&1)<<2 | int64(h>>3&1)<<1
			imm = (imm << 52) >> 52 // sign extend 12 bits
			tgt := p.addrsToSlot(uint64(int64(addr) + imm))
			if tgt < 0 {
				return p.fault("C.J target")
			}
			d := tgt - self
			return func(vm *VM) int {
				return d
			}
		}
		return p.fault("C reserved h=" + sprintf("0x%x q=%d f3=%d", h, op, f3))

	case 2: // Quadrant 2
		switch f3 {
		case 0: // C.SLLI
			sh := uint64(h>>12&1)<<5 | uint64(h>>2&0x1f)
			return func(vm *VM) int {
				vm.reg(rd, vm.x[rd]<<sh)
				return 1
			}
		case 1: // C.FLDSP: rd at [11:7], imm6 at [12]+[6:2], scale 8
			imm := uint64(h>>12&1)<<5 | uint64(h>>2&0x1f)
			imm <<= 3
			if rd == 0 {
				return p.fault("C.FLDSP x0")
			}
			return func(vm *VM) int {
				vm.f[rd] = vm.mem.ReadU64(vm.x[2] + imm)
				return 1
			}
		case 2: // C.LWSP: rd at [11:7], imm6 at [12]+[6:2], scale 4
			imm := uint64(h>>12&1)<<5 | uint64(h>>2&0x1f)
			imm <<= 2
			if rd == 0 {
				return p.fault("C.LWSP x0")
			}
			return func(vm *VM) int {
				vm.reg(rd, uint64(int64(int32(vm.mem.ReadU32(vm.x[2]+imm)))))
				return 1
			}
		case 3: // C.LDSP: rd at [11:7], imm6 at [12]+[6:2], scale 8
			imm := uint64(h>>12&1)<<5 | uint64(h>>2&0x1f)
			imm <<= 3
			if rd == 0 {
				return p.fault("C.LDSP x0")
			}
			return func(vm *VM) int {
				vm.reg(rd, vm.mem.ReadU64(vm.x[2]+imm))
				return 1
			}
		case 4: // C.JR / C.MV / C.EBREAK / C.JALR / C.ADD
			if h>>12&1 == 0 {
				if rs2 == 0 { // C.JR
					if rd == 0 {
						return p.fault("C.JR x0")
					}
					return func(vm *VM) int {
						t := vm.x[rd] &^ 1
						if t == sigreturnSentinel {
							vm.doSigReturn()
							return 0
						}
						ts := p.addrsToSlot(t)
						if ts < 0 {
							vm.fault("C.JR target")
							return 1
						}
						return ts - self
					}
				}
				// C.MV
				return func(vm *VM) int {
					vm.reg(rd, vm.x[rs2])
					return 1
				}
			}
			if rs2 == 0 {
				if rd == 0 { // C.EBREAK
					return func(vm *VM) int {
						vm.trap = trapEbreak
						return 1
					}
				}
				// C.JALR
				rdv := uint64(addr + 2)
				return func(vm *VM) int {
					t := vm.x[rd] &^ 1
					if t == sigreturnSentinel {
						// Only meaningful as a return (rd == 0); treat as sigreturn.
						vm.reg(rd, rdv)
						vm.doSigReturn()
						return 0
					}
					ts := p.addrsToSlot(t)
					if ts < 0 {
						vm.fault("C.JALR target")
						return 1
					}
					vm.reg(rd, rdv)
					return ts - self
				}
			}
			// C.ADD
			return func(vm *VM) int {
				vm.reg(rd, vm.x[rd]+vm.x[rs2])
				return 1
			}
		case 5: // C.FSDSP (uimm[5]<<5 | uimm[4:3]<<3 | uimm[8:6]<<6)
			// Register field [6:2] is rs2; bits[11:7] are the immediate,
			// so there is no rd to reserve for x0.
			imm := uint64(h>>12&1)<<5 | uint64(h>>11&1)<<4 | uint64(h>>10&1)<<3 | uint64(h>>9&1)<<8 | uint64(h>>8&1)<<7 | uint64(h>>7&1)<<6
			return func(vm *VM) int {
				vm.mem.WriteU64(vm.x[2]+imm, vm.f[rs2])
				return 1
			}
		case 6: // C.SWSP (uimm[5]<<5 | uimm[4:2]<<2 | uimm[7:6]<<6)
			imm := uint64(h>>12&1)<<5 | uint64(h>>11&1)<<4 | uint64(h>>10&1)<<3 | uint64(h>>9&1)<<2 | uint64(h>>8&1)<<7 | uint64(h>>7&1)<<6
			return func(vm *VM) int {
				vm.mem.WriteU32(vm.x[2]+imm, uint32(vm.x[rs2]))
				return 1
			}
		case 7: // C.SDSP (uimm[5]<<5 | uimm[4:3]<<3 | uimm[8:6]<<6)
			imm := uint64(h>>12&1)<<5 | uint64(h>>11&1)<<4 | uint64(h>>10&1)<<3 | uint64(h>>9&1)<<8 | uint64(h>>8&1)<<7 | uint64(h>>7&1)<<6
			return func(vm *VM) int {
				vm.mem.WriteU64(vm.x[2]+imm, vm.x[rs2])
				return 1
			}
		}
		return p.fault("C reserved h=" + sprintf("0x%x q=%d f3=%d", h, op, f3))
	}
	return p.fault("C op")
}

func sext5(v int64) int64 { return (v << 58) >> 58 }
func sext9(v int64) int64 { return (v << 54) >> 54 }
