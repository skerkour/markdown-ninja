package riscvemu

import (
	"time"
)

// VM is one emulated thread: a register file plus an instruction pointer into
// the shared compiled program. Memory and code are shared across threads.
type VM struct {
	m     *Machine
	mem   *Mem
	x     [32]uint64
	f     [32]uint64 // FP registers as raw bits
	fcsr  uint32     // fflags:frm:fcsr (keep bits: fflags=0x1f, frm=0xe0)
	pc    int        // slot index into program
	trap  int
	state int

	// scheduling/park state
	parkAddr uint64
	parkVal  uint32
	wakeTime time.Time
	futexRet int64 // futex syscall return to apply when woken

	tid           uint32
	clearChildTid uint64
	clearChildSet bool

	yield  bool
	exited bool

	// signal delivery state
	sigPending uint32 // pending signal number, 0 = none
	inSig      bool   // currently inside a signal handler
	sigSave    *savedSig
	sigStack   sigStack
	sigMask    uint64

	// trace of recent execution for debugging
	trace []uint64
}

// traceN keeps the last n executed addresses (debug aid).
func (vm *VM) traceAdd() {
	if vm.m.trace {
		vm.trace = append(vm.trace, vm.m.prog.addr(vm.pc))
		if len(vm.trace) > 4096 {
			vm.trace = vm.trace[len(vm.trace)-4096:]
		}
	}
}

// thread states
const (
	stateRunnable = iota
	stateBlockedFutex
	stateBlockedSleep
	stateExited
)

// Machine owns the compiled program, shared memory, the syscall shim and the
// cooperative thread scheduler.
type Machine struct {
	prog *program
	mem  *Mem
	sys  *Syscalls

	threads []*VM
	cur     int
	futex   map[uint64][]*VM
	nextTid uint32

	exited   bool
	exitCode int

	// mmap allocation state
	regions    []region
	mmapCursor uint64
	stackStart uint64

	// initial auxv image (for /proc/self/auxv)
	auxvBytes []byte

	// debug tracing
	trace        bool
	traceSys     bool
	traceSp      bool
	minSp        uint64
	msCount      int
	sawThrow     bool
	loopIters    int
	lastPc       uint64
	sawPark      bool
	parkCount    int
	log1000      int
	newprocCount int
	sawGcenable  bool
	mmapCount    int

	// temporary preemption trace aid
	tracePreempt    bool
	tracePreemptTid uint32
	tracePreemptN   int

	// monotonic clock origin for guest clock_gettime
	start time.Time

	// networking wakeup: background poller signals when a host socket is ready
	netWake    chan struct{}
	netPollers int
}

func NewMachine(prog *program, mem *Mem) *Machine {
	m := &Machine{
		prog:       prog,
		mem:        mem,
		sys:        NewSyscalls(),
		futex:      make(map[uint64][]*VM),
		nextTid:    1000,
		mmapCursor: 0x0000004000000000, // 2^38, grows down (below Sv48 user limit 2^47)
		start:      time.Now(),
		netWake:    make(chan struct{}, 1),
	}
	m.sys.m = m
	return m
}

func (m *Machine) newThread(entry int) *VM {
	t := &VM{m: m, mem: m.mem, pc: entry, state: stateRunnable, tid: m.nextTid}
	m.nextTid++
	m.threads = append(m.threads, t)
	return t
}

func (vm *VM) reg(r int, v uint64) {
	if r != 0 {
		vm.x[r] = v
	}
}

func (vm *VM) fault(msg string) {
	vm.m.exitCode = -1
	vm.exited = true
	vm.state = stateExited
	panic(&Fault{Msg: msg, PC: vm.m.prog.addr(vm.pc), Trace: append([]uint64{}, vm.trace...), Regs: vm.x, FRegs: vm.f, TID: vm.tid})
}

// Fault is raised when the guest traps or hits an unsupported instruction.
type Fault struct {
	Msg   string
	PC    uint64
	Trace []uint64
	Regs  [32]uint64
	FRegs [32]uint64
	TID   uint32
}

func (f *Fault) Error() string { return f.Msg }

// csr reads a CS register. Only the FP CSRs are meaningful in user space.
func (vm *VM) csr(i int) uint64 {
	switch i {
	case 0x001: // fflags
		return uint64(vm.fcsr & 0x1f)
	case 0x002: // frm
		return uint64((vm.fcsr >> 5) & 0x7)
	case 0x003: // fcsr
		return uint64(vm.fcsr & 0xff)
	case 0x7f1: // fscratch
		return 0
	}
	return 0
}

func (vm *VM) setcsr(i int, v uint64) {
	switch i {
	case 0x001:
		vm.fcsr = (vm.fcsr &^ 0x1f) | uint32(v&0x1f)
	case 0x002:
		vm.fcsr = (vm.fcsr &^ 0xe0) | uint32((v&0x7)<<5)
	case 0x003:
		vm.fcsr = uint32(v & 0xff)
	}
}

// rounding mode from fcsr
func (vm *VM) rm() int { return int((vm.fcsr >> 5) & 0x7) }

// setfflags ORs the given exception flags into fcsr.
func (vm *VM) setfflags(f uint32) { vm.fcsr |= f }

// Run advances the thread until it traps, blocks, yields, or consumes
// maxClosures instructions. The caller must dispatch the pending trap.
func (vm *VM) Run(maxClosures int) {
	for i := 0; i < maxClosures; i++ {
		if vm.pc < 0 || vm.pc >= len(vm.m.prog.code) {
			vm.fault("pc out of program")
		}
		// Deliver a pending asynchronous signal (e.g. SIGURG for async
		// preemption) before executing the next instruction.
		if vm.sigPending != 0 && !vm.inSig {
			if vm.deliverPending() {
				continue
			}
		}
		vm.traceAdd()
		vm.dbgMorestack()
		vm.dbgPreemptTrace()
		delta := vm.m.prog.code[vm.pc](vm)
		if vm.exited {
			return
		}
		if vm.trap != trapNone {
			return // pc still points at the trap instruction
		}
		if vm.yield {
			vm.pc += delta
			vm.yield = false
			return
		}
		vm.pc += delta
	}
}

const runQuantum = 100000

// Schedule runs all threads cooperatively until the process exits.
func (m *Machine) Schedule() (exitCode int, err error) {
	defer func() {
		if r := recover(); r != nil {
			if f, ok := r.(*Fault); ok {
				err = f
				return
			}
			panic(r)
		}
	}()

	for !m.exited {
		ran := false
		for i := 0; i < len(m.threads) && !m.exited; i++ {
			t := m.threads[i]
			if t.exited || t.state != stateRunnable {
				continue
			}
			ran = true
			t.Run(runQuantum)
			if t.exited {
				continue
			}
			if t.trap == trapEcall {
				t.trap = trapNone
				// Consume the 32-bit ecall before dispatching so that a
				// syscall which blocks the thread (futex-wait, nanosleep)
				// resumes correctly after the ecall when the thread is
				// woken, instead of re-executing the ecall with the
				// syscall return value in a0.
				t.pc += 2
				m.sys.Dispatch(t)
			} else if t.trap == trapEbreak {
				t.fault("ebreak")
			}
			switch t.state {
			case stateBlockedFutex, stateBlockedSleep:
				// keep parked
			}
		}
		// Wake any timed freight (futex wait or sleep) whose deadline passed.
		woke := m.wakeExpired()
		// If a host socket became ready, wake threads parked in epoll_pwait
		// so they re-poll and observe the readiness.
		select {
		case <-m.netWake:
			for _, t := range m.threads {
				if t.state == stateBlockedSleep && !t.wakeTime.IsZero() {
					t.wakeTime = time.Time{}
					t.state = stateRunnable
					woke = true
				}
			}
		default:
		}
		if !ran && !woke {
			// all threads blocked and nothing has expired; sleep until the
			// earliest wakeup deadline.
			if !m.advanceTime() {
				break
			}
		}
	}
	return m.exitCode, nil
}

// wakeExpired wakes blocked threads (futex waits and sleeps) whose wakeTime
// deadline has passed. Returns true if any thread was woken.
func (m *Machine) wakeExpired() bool {
	now := time.Now()
	woke := false
	for _, t := range m.threads {
		if t.exited {
			continue
		}
		if (t.state == stateBlockedFutex || t.state == stateBlockedSleep) && !t.wakeTime.IsZero() && !t.wakeTime.After(now) {
			if t.state == stateBlockedFutex {
				t.futexRet = errETIMEDOUT
				t.x[10] = neg(errETIMEDOUT)
			}
			t.wakeTime = time.Time{}
			t.state = stateRunnable
			woke = true
		}
	}
	return woke
}

// advanceTime wakes threads whose sleep deadline passed, or sleeps a while.
// Returns false if no progress is possible (deadlock).
func (m *Machine) advanceTime() bool {
	var soonest time.Time
	any := false
	for _, t := range m.threads {
		if t.state == stateBlockedSleep && !t.wakeTime.IsZero() {
			if !any || t.wakeTime.Before(soonest) {
				soonest = t.wakeTime
			}
			any = true
		}
	}
	if !any {
		return false
	}
	now := time.Now()
	if soonest.After(now) {
		// sleep until the earliest deadline, or wake early if a host socket
		// became ready (netWake).
		d := soonest.Sub(now)
		select {
		case <-m.netWake:
		case <-time.After(d):
		}
	}
	for _, t := range m.threads {
		if t.state == stateBlockedSleep && !t.wakeTime.IsZero() && !t.wakeTime.After(time.Now()) {
			t.state = stateRunnable
		}
	}
	return true
}

// park parks the current thread.
func (vm *VM) park(state int) { vm.state = state }

// sleepUntil blocks the thread until t.
func (vm *VM) sleepUntil(t time.Time) {
	vm.wakeTime = t
	vm.state = stateBlockedSleep
}

// futexWait parks the thread waiting on addr with the given expected value.
// deadline zero means wait indefinitely. The syscall return value is set by
// the waker (0 on FUTEX_WAKE, -ETIMEDOUT on timeout).
func (vm *VM) futexWait(addr uint64, val uint32, deadline time.Time) {
	vm.parkAddr = addr
	vm.parkVal = val
	vm.wakeTime = deadline
	vm.futexRet = 0
	vm.m.futex[addr] = append(vm.m.futex[addr], vm)
	vm.state = stateBlockedFutex
}

// futexWaitUntil is kept for clarity; it delegates to futexWait.
func (vm *VM) futexWaitUntil(addr uint64, val uint32, deadline time.Time) {
	vm.futexWait(addr, val, deadline)
}

// futexWake wakes up to n waiters on addr. Returns count woken.
func (m *Machine) futexWake(addr uint64, n int) int {
	waiters := m.futex[addr]
	if len(waiters) == 0 {
		return 0
	}
	woken := 0
	for i := 0; i < len(waiters) && woken < n; i++ {
		w := waiters[i]
		if w.state == stateBlockedFutex {
			w.state = stateRunnable
			w.wakeTime = time.Time{}
			w.futexRet = 0
			w.x[10] = 0
			woken++
		}
	}
	if woken > 0 {
		m.futex[addr] = waiters[woken:]
	}
	return woken
}
