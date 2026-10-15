package riscvemu

import (
	"fmt"
	"os"
)

func (vm *VM) dbgMorestack() {}

// dbgPreemptTrace prints the pc of a designated thread for a few instructions
// after a signal is delivered, to debug async-preemption handler execution.
func (vm *VM) dbgPreemptTrace() {
	if vm.m.tracePreempt && vm.tid == vm.m.tracePreemptTid && vm.m.tracePreemptN > 0 {
		fmt.Fprintf(os.Stderr, "  preempt pc=0x%x sp=0x%x\n", vm.m.prog.addr(vm.pc), vm.x[2])
		vm.m.tracePreemptN--
	}
}

func (s *Syscalls) dbgSyscall(vm *VM) {
	if !vm.m.traceSys {
		return
	}
	num := vm.x[17]
	a := vm.x[:]
	switch num {
	case 220:
		fmt.Fprintf(os.Stderr, "t%d clone(flags=0x%x,stk=0x%x)\n", vm.tid, a[10], a[11])
	case 98:
		fmt.Fprintf(os.Stderr, "t%d futex(0x%x,op=0x%x,val=%d,tmo=0x%x)\n", vm.tid, a[10], a[11], int64(a[12]), a[13])
	case 93:
		fmt.Fprintf(os.Stderr, "t%d exit\n", vm.tid)
	case 94:
		fmt.Fprintf(os.Stderr, "t%d exit_group\n", vm.tid)
	case 101:
		fmt.Fprintf(os.Stderr, "t%d nanosleep(%d,%d)\n", vm.tid, int64(vm.mem.ReadU64(a[10])), int64(vm.mem.ReadU64(a[10]+8)))
	case 131:
		fmt.Fprintf(os.Stderr, "t%d tgkill(tgid=%d,tid=%d,sig=%d)\n", vm.tid, int64(a[10]), int64(a[11]), int64(a[12]))
	case 139:
		fmt.Fprintf(os.Stderr, "t%d rt_sigreturn\n", vm.tid)
	case 134:
		fmt.Fprintf(os.Stderr, "t%d rt_sigaction(sig=%d,new=0x%x,old=0x%x)\n", vm.tid, int64(a[10]), a[11], a[12])
	case 132:
		fmt.Fprintf(os.Stderr, "t%d sigaltstack(new=0x%x,old=0x%x)\n", vm.tid, a[10], a[11])
	case 64:
		if a[10] == 1 || a[10] == 2 || a[10] >= 3 {
			fmt.Fprintf(os.Stderr, "t%d write(%d,0x%x,%d)\n", vm.tid, int64(a[10]), a[11], int64(a[12]))
		}
	case 198:
		fmt.Fprintf(os.Stderr, "t%d socket(domain=%d,type=%d,proto=%d)\n", vm.tid, int64(a[10]), int64(a[11]), int64(a[12]))
	case 203:
		fmt.Fprintf(os.Stderr, "t%d connect(fd=%d)\n", vm.tid, int64(a[10]))
	case 206:
		fmt.Fprintf(os.Stderr, "t%d sendto(fd=%d,n=%d)\n", vm.tid, int64(a[10]), int64(a[12]))
	case 207:
		fmt.Fprintf(os.Stderr, "t%d recvfrom(fd=%d,n=%d)\n", vm.tid, int64(a[10]), int64(a[12]))
	case 211:
		fmt.Fprintf(os.Stderr, "t%d sendmsg(fd=%d)\n", vm.tid, int64(a[10]))
	case 212:
		fmt.Fprintf(os.Stderr, "t%d recvmsg(fd=%d)\n", vm.tid, int64(a[10]))
	case 63:
		if a[10] >= 3 {
			fmt.Fprintf(os.Stderr, "t%d read(fd=%d,n=%d)\n", vm.tid, int64(a[10]), int64(a[12]))
		}
	case 20:
		fmt.Fprintf(os.Stderr, "t%d epoll_create1\n", vm.tid)
	case 21:
		fmt.Fprintf(os.Stderr, "t%d epoll_ctl(epfd=%d,op=%d,fd=%d)\n", vm.tid, int64(a[10]), int64(a[11]), int64(a[12]))
	case 22:
		fmt.Fprintf(os.Stderr, "t%d epoll_pwait\n", vm.tid)
	}
}
