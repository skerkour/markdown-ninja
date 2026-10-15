package riscvemu

// Signal handling and asynchronous preemption.
//
// The Go runtime preempts goroutines asynchronously by sending SIGURG to a
// target thread via tgkill. To run multi-P programs (GOMAXPROCS > 1) where a
// busy-loop goroutine occupies a P with no cooperative preemption point, the
// emulator must actually deliver that signal: save the thread's context into a
// guest-visible ucontext, jump into the installed handler (runtime·sigtramp),
// and restore the (possibly modified) ucontext when the handler returns.

import (
	"fmt"
	"os"
)

// sa_flags bits
const (
	saOnstack = 0x08000000
	siTkill   = uint32(0xfffffffa) // -6 (SI_TKILL)
)

// Signal types.
type sigaction struct {
	handler uint64
	flags   uint64
	mask    uint64
}

// sigreturnSentinel is a synthetic return address used as the sigtramp return
// address. When the guest RET/JALR returns to it, the emulator intercepts it
// and performs the signal-context restore. It is a 2-byte-aligned address far
// outside the program text and any mapped region.
const sigreturnSentinel = 0x0000fffffffffff0

// Offsets into ucontext (mirrors runtime's defs for riscv64).
//
//	ucontext {
//	  uc_flags       u64    @ 0
//	  uc_link        u64    @ 8
//	  uc_stack       stackt @ 16  (ss_sp u64@16, ss_flags i32@24, ss_size u64@32)
//	  uc_sigmask     [128]  @ 40
//	  uc_pad_cgo_0   [8]    @ 168
//	  uc_mcontext    sigcontext @ 176
//	}
//
//	sigcontext {
//	  sc_regs        user_regs_struct @ 0  (256 bytes, 32 x u64)
//	  sc_fpregs      [528]            @ 256
//	}
const (
	ucontextOffset   = 0
	ucMcontextOffset = 176
	scRegsOffset     = ucMcontextOffset + 0   // 176
	scFpregsOffset   = ucMcontextOffset + 256 // 432
	siginfoSize      = 128
	signalFrameSize  = 1024 + siginfoSize // room for ucontext + siginfo
)

// user_regs_struct field order (relative to sc_regs).
// pc, ra, sp, gp, tp, t0..t2, s0, s1, a0..a7, s2..s11, t3..t6
var regFieldToX = []struct {
	name byte
	x    int
}{
	{'p', -1},                    // pc (special)
	{'r', 1},                     // ra
	{'s', 2},                     // sp
	{'g', 3},                     // gp
	{'t', 4},                     // tp
	{'t', 5}, {'t', 6}, {'t', 7}, // t0,t1,t2
	{'s', 8}, {'s', 9}, // s0,s1
	{'a', 10}, {'a', 11}, {'a', 12}, {'a', 13}, {'a', 14}, {'a', 15}, {'a', 16}, {'a', 17}, // a0..a7
	{'s', 18}, {'s', 19}, {'s', 20}, {'s', 21}, {'s', 22}, {'s', 23}, {'s', 24}, {'s', 25}, {'s', 26}, {'s', 27}, // s2..s11
	{'t', 28}, {'t', 29}, {'t', 30}, {'t', 31}, // t3..t6
}

// writeUcontextRegs writes the thread's GP registers and pc into the guest
// ucontext's sc_regs at address base.
func (vm *VM) writeUcontextRegs(base uint64) {
	for i, f := range regFieldToX {
		off := base + uint64(i)*8
		if f.x < 0 {
			vm.memu().WriteU64(off, vm.pcU())
			continue
		}
		vm.memu().WriteU64(off, vm.x[f.x])
	}
}

// readUcontextRegs restores the thread's GP registers and pc from the guest
// ucontext's sc_regs at address base.
func (vm *VM) readUcontextRegs(base uint64) {
	for i, f := range regFieldToX {
		off := base + uint64(i)*8
		v := vm.memu().ReadU64(off)
		if f.x < 0 {
			vm.setPC(v)
			continue
		}
		vm.reg(f.x, v)
	}
}

// pcU returns the current pc as a guest address.
func (vm *VM) pcU() uint64 { return vm.m.prog.addr(vm.pc) }

// setPC sets the instruction-pointer slot from a guest address.
func (vm *VM) setPC(a uint64) {
	ts := vm.m.prog.addrsToSlot(a)
	if ts < 0 {
		vm.fault("sigreturn pc outside program")
		return
	}
	vm.pc = ts
}

// memu is a shorthand for the shared memory.
func (vm *VM) memu() *Mem { return vm.mem }

// savedSig holds the host-side copy of a preempted thread's context.
type savedSig struct {
	x    [32]uint64
	f    [32]uint64
	fcsr uint32
	ctx  uint64 // guest address of the ucontext for this delivery
}

// alt stack state (per thread)
type sigStack struct {
	sp      uint64
	size    uint64
	onstack bool
}

// traceSys2 reports whether syscall tracing is enabled.
func (vm *VM) traceSys2() bool { return vm.m.traceSys }

// deliverPending delivers the thread's pending signal if any. Called at a safe
// instruction boundary in Run. Returns true if a signal was delivered.
func (vm *VM) deliverPending() bool {
	if vm.traceSys2() {
		fmt.Fprintf(os.Stderr, "t%d DELIVER sig=%d inSig=%v\n", vm.tid, vm.sigPending, vm.inSig)
	}
	if vm.sigPending == 0 || vm.inSig {
		return false
	}
	sa, ok := vm.m.sys.sigactions[vm.sigPending]
	if !ok || sa.handler == 0 {
		// no handler installed; just clear and ignore
		vm.sigPending = 0
		return false
	}

	// Save current context.
	saved := &savedSig{
		x:    vm.x,
		f:    vm.f,
		fcsr: vm.fcsr,
	}

	// Determine the stack to run the handler on.
	sp := vm.x[2]
	onAlt := false
	if sa.flags&saOnstack != 0 && vm.sigStack.sp != 0 && vm.sigStack.size != 0 && !vm.sigStack.onstack {
		// Switch to the alternate signal stack. Go registers it with ss_sp
		// as the low (base) address and ss_size covering [ss_sp, ss_sp+size);
		// the stack grows down from the top, so enter near the top.
		sp = vm.sigStack.sp + vm.sigStack.size
		onAlt = true
	}

	// Build the frame just below the entry sp (within the alt stack range).
	frame := sp - signalFrameSize
	if onAlt && frame < vm.sigStack.sp {
		frame = vm.sigStack.sp
	}
	frame &^= 15 // align down to 16
	infoAddr := frame
	ctxAddr := frame + siginfoSize

	// siginfo
	vm.mem.WriteU32(infoAddr, vm.sigPending)     // si_signo
	vm.mem.WriteU32(infoAddr+4, 0)               // si_errno
	vm.mem.WriteU32(infoAddr+8, uint32(siTkill)) // si_code (SI_TKILL)
	vm.mem.WriteU64(infoAddr+16, 0)              // si_pid  (in union)

	// ucontext: write regs into sc_regs
	vm.writeUcontextRegs(ctxAddr + scRegsOffset)

	saved.ctx = ctxAddr
	saved.fcsr = vm.fcsr

	// Set up handler entry.
	vm.sigSave = saved
	vm.inSig = true
	sig := vm.sigPending
	vm.sigPending = 0

	if vm.m.tracePreempt && vm.tid == vm.m.tracePreemptTid {
		fmt.Fprintf(os.Stderr, "  DELIVERING sig=%d -> handler=0x%x sp=0x%x frame=0x%x\n", sig, sa.handler, sp, frame)
		vm.m.tracePreemptN = 4000
	}

	vm.x[10] = uint64(sig)      // a0 = signo
	vm.x[11] = infoAddr         // a1 = siginfo
	vm.x[12] = ctxAddr          // a2 = ucontext
	vm.x[2] = frame             // sp
	vm.x[1] = sigreturnSentinel // ra = sigreturn trampoline
	vm.setPC(sa.handler)        // pc = sigtramp
	if onAlt {
		vm.sigStack.onstack = true
	}
	return true
}

// doSigReturn restores the preempted context after the signal handler returns.
func (vm *VM) doSigReturn() {
	saved := vm.sigSave
	if saved == nil {
		vm.fault("sigreturn without saved context")
		return
	}
	// Restore GP regs + pc from the (possibly modified) ucontext.
	vm.readUcontextRegs(saved.ctx + scRegsOffset)
	// Restore FP regs from the host copy (Go's pushCall only mutates GP/pc).
	vm.f = saved.f
	vm.fcsr = saved.fcsr
	vm.sigSave = nil
	vm.inSig = false
	if vm.sigStack.onstack {
		vm.sigStack.onstack = false
	}
	if vm.sigPending != 0 {
		vm.deliverPending()
	}
}

// --- syscall shims ------------------------------------------------------------

// rtSigaction implements syscall 134.
func (s *Syscalls) rtSigaction(vm *VM, sig, new, old, size uint64) int64 {
	sig32 := uint32(sig)
	if sig32 == 0 || sig32 > 64 {
		return int64(-errEINVAL)
	}
	if old != 0 {
		existing := s.sigactions[sig32]
		var handler, flags, mask uint64
		if existing != nil {
			handler, flags, mask = existing.handler, existing.flags, existing.mask
		}
		vm.mem.WriteU64(old, handler)
		vm.mem.WriteU64(old+8, flags)
		vm.mem.WriteU64(old+16, 0) // sa_restorer (0 on riscv64)
		vm.mem.WriteU64(old+24, mask)
	}
	if new != 0 {
		s.sigactions[sig32] = &sigaction{
			handler: vm.mem.ReadU64(new),
			flags:   vm.mem.ReadU64(new + 8),
			mask:    vm.mem.ReadU64(new + 24),
		}
	}
	_ = size
	return 0
}

// rtSigprocmask: mask is tracked but SIGURG is never blocked in practice.
func (s *Syscalls) rtSigprocmask(vm *VM, how, new, old, size uint64) int64 {
	if old != 0 {
		vm.mem.WriteU64(old, 0)
	}
	if new != 0 {
		vm.sigMask = vm.mem.ReadU64(new)
	}
	_ = how
	_ = size
	return 0
}

// sigaltstack implements syscall 132. a0 = new stackt ptr, a1 = old stackt ptr.
func (s *Syscalls) sigaltstack(vm *VM, newst, oldst uint64) int64 {
	if oldst != 0 {
		vm.mem.WriteU64(oldst, vm.sigStack.sp)      // ss_sp
		vm.mem.WriteU32(oldst+8, 0)                 // ss_flags
		vm.mem.WriteU64(oldst+16, vm.sigStack.size) // ss_size
	}
	if newst != 0 {
		sp := vm.mem.ReadU64(newst)
		flags := int32(vm.mem.ReadU32(newst + 8))
		size := vm.mem.ReadU64(newst + 16)
		if flags&0x2 != 0 { // SS_DISABLE
			vm.sigStack = sigStack{}
		} else {
			vm.sigStack.sp = sp
			vm.sigStack.size = size
		}
	}
	return 0
}

// tgkill: deliver sig to the thread with the given tid. syscall 131.
func (s *Syscalls) tgkill(vm *VM, tgid, tid, sig uint64) int64 {
	_ = tgid
	return s.deliverSignal(tid, uint32(sig))
}

func (s *Syscalls) tkill(vm *VM, tid, sig uint64) int64 {
	return s.deliverSignal(tid, uint32(sig))
}

func (s *Syscalls) raise(vm *VM, sig uint64) int64 {
	return s.deliverSignal(uint64(vm.tid), uint32(sig))
}

// killerImpl delivers sig to the named process/kill target.
func (s *Syscalls) kill(vm *VM, pid, sig uint64) int64 {
	if pid == 0 || pid == uint64(s.pid) {
		return s.deliverSignal(uint64(vm.tid), uint32(sig))
	}
	_ = vm
	if t := s.findThread(uint32(pid)); t != nil {
		return s.deliverSignal(pid, uint32(sig))
	}
	return 0
}

// deliverSignal targets the thread with the given tid.
func (s *Syscalls) deliverSignal(tid uint64, sig uint32) int64 {
	tgt := s.findThread(uint32(tid))
	if tgt == nil {
		return int64(-errEAGAIN) // ESRCH, close enough
	}
	if sig == 0 {
		return 0
	}
	tgt.sigPending = sig
	return 0
}

func (s *Syscalls) findThread(tid uint32) *VM {
	for _, t := range s.m.threads {
		if t.tid == tid {
			return t
		}
	}
	return nil
}

// rtSigreturn restores the preempted context. syscall 139.
func (s *Syscalls) rtSigreturn(vm *VM) int64 {
	vm.doSigReturn()
	return 0
}
