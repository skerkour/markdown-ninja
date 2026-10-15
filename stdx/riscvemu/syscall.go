package riscvemu

import (
	"crypto/rand"
	"os"
	"time"
)

// Syscalls implements the riscv64 Linux user-visible syscall surface.
type Syscalls struct {
	m *Machine

	stdin  *os.File
	stdout *os.File
	stderr *os.File

	fds    map[int]*vfile
	nextFD int

	breakAddr uint64 // brk

	pid    int
	nchild int

	// signal handler table
	sigactions map[uint32]*sigaction

	socks    map[int]*sock
	epollfds map[int]struct{}
	epollReg map[int]map[uint64]epollReg // epfd -> fd -> registration
	pipes    map[int]*os.File
	eventfds map[int]*pipePair
}

// epollReg records one registered fd on an epoll instance.
type epollReg struct {
	fd   uint64
	data uint64
	ev   uint32
}

const (
	sysEAGAIN = 11
	sysENOENT = 2
	sysEINVAL = 22
)

func NewSyscalls() *Syscalls {
	s := &Syscalls{
		stdin:      os.Stdin,
		stdout:     os.Stdout,
		stderr:     os.Stderr,
		fds:        make(map[int]*vfile),
		nextFD:     3,
		sigactions: make(map[uint32]*sigaction),
		socks:      make(map[int]*sock),
		epollfds:   make(map[int]struct{}),
		epollReg:   make(map[int]map[uint64]epollReg),
		pipes:      make(map[int]*os.File),
		eventfds:   make(map[int]*pipePair),
	}
	// stdio
	s.fds[0] = &vfile{fd: 0, name: "stdin"}
	s.fds[1] = &vfile{fd: 1, name: "stdout"}
	s.fds[2] = &vfile{fd: 2, name: "stderr"}
	return s
}

// Dispatch handles a syscall for the current thread.
// Syscall number in x17, args in x10..x15, return in x10.
func (s *Syscalls) Dispatch(vm *VM) {
	s.dbgSyscall(vm)
	a := vm.x[:]
	num := a[17]

	switch num {
	case 56: // openat
		dirfd := int(int64(a[10]))
		name := vm.mem.ReadCString(a[11])
		flags := a[12]
		_ = dirfd
		_ = flags
		fd := s.doOpen(vm, name)
		a[10] = uint64(int64(fd))
	case 57: // close
		fd := int(int64(a[10]))
		s.doClose(fd)
		a[10] = 0
	case 63: // read
		fd := int(int64(a[10]))
		buf := a[11]
		n := int(a[12])
		r := s.doRead(vm, fd, buf, n)
		a[10] = uint64(int64(r))
	case 64: // write
		fd := int(int64(a[10]))
		buf := a[11]
		n := int(a[12])
		r := s.doWrite(vm, fd, buf, n)
		a[10] = uint64(int64(r))
	case 67: // pread64
		fd := int(int64(a[10]))
		buf := a[11]
		n := int(a[12])
		off := int64(a[13])
		r := s.doPread(vm, fd, buf, n, off)
		a[10] = uint64(int64(r))
	case 62: // lseek
		fd := int(int64(a[10]))
		off := int64(a[11])
		whence := int(a[12])
		r := s.doLseek(fd, off, whence)
		a[10] = uint64(int64(r))
	case 25: // fcntl
		a[10] = 0
	case 222: // mmap
		addr := a[10]
		length := a[11]
		prot := a[12]
		flags := a[13]
		fd := int(int64(a[14]))
		off := a[15]
		r := s.m.mmap(vm, addr, length, prot, flags, fd, off)
		a[10] = r
	case 215: // munmap
		s.m.munmap(a[10], a[11])
		a[10] = 0
	case 226: // mprotect
		s.m.mprotect(a[10], a[11], a[12])
		a[10] = 0
	case 233: // madvise
		a[10] = 0
	case 232: // mincore
		addr := a[10]
		length := a[11]
		vec := a[12]
		s.m.mincore(addr, length, vec)
		a[10] = 0
	case 214: // brk
		a[10] = s.doBrk(a[10])
	case 220: // clone
		flags := a[10]
		stk := a[11]
		s.doClone(vm, flags, stk)
	case 98: // futex
		addr := a[10]
		op := int64(a[11])
		val := uint32(a[12])
		timeout := a[13]
		s.doFutex(vm, addr, op, val, timeout)
	case 178: // gettid
		a[10] = uint64(vm.tid)
	case 172: // getpid
		a[10] = uint64(s.pid)
	case 173: // getppid
		a[10] = uint64(s.pid - 1)
	case 174: // getuid
		a[10] = 0
	case 176: // getgid
		a[10] = 0
	case 123: // sched_getaffinity
		pid := int(int64(a[10]))
		size := a[11]
		buf := a[12]
		_ = pid
		// one CPU: single bit set
		if size > 0 {
			vm.mem.WriteU8(buf, 1)
		}
		a[10] = 8
	case 124: // sched_yield
		vm.yield = true
		a[10] = 0
	case 134: // rt_sigaction
		a[10] = uint64(s.rtSigaction(vm, a[10], a[11], a[12], a[13]))
	case 135: // rt_sigprocmask
		a[10] = uint64(s.rtSigprocmask(vm, a[10], a[11], a[12], a[13]))
	case 132: // sigaltstack
		a[10] = uint64(s.sigaltstack(vm, a[10], a[11]))
	case 131: // tgkill
		a[10] = uint64(s.tgkill(vm, a[10], a[11], a[12]))
	case 130: // tkill
		a[10] = uint64(s.tkill(vm, a[10], a[11]))
	case 129: // kill
		a[10] = uint64(s.kill(vm, a[10], a[11]))
	case 139: // rt_sigreturn
		a[10] = uint64(s.rtSigreturn(vm))
	case 113: // clock_gettime
		clock := int64(a[10])
		tp := a[11]
		var sec, nsec int64
		switch clock {
		case 0: // CLOCK_REALTIME
			now := time.Now()
			sec = now.Unix()
			nsec = int64(now.Nanosecond())
		case 1, 2: // MONOTONIC / MONOTONIC_RAW
			d := time.Since(s.m.start)
			sec = int64(d.Seconds())
			nsec = int64(d.Nanoseconds() % 1e9)
		default:
			a[10] = neg(errEINVAL)
			return
		}
		vm.mem.WriteU64(tp, uint64(sec))
		vm.mem.WriteU64(tp+8, uint64(nsec))
		a[10] = 0
	case 101: // nanosleep
		req := a[10]
		rem := a[11]
		sec := int64(vm.mem.ReadU64(req))
		nsec := int64(vm.mem.ReadU64(req + 8))
		d := time.Duration(sec)*time.Second + time.Duration(nsec)
		vm.sleepUntil(time.Now().Add(d))
		if rem != 0 {
			vm.mem.WriteU64(rem, 0)
			vm.mem.WriteU64(rem+8, 0)
		}
		a[10] = 0
	case 169: // gettimeofday
		tv := a[10]
		now := time.Now()
		vm.mem.WriteU64(tv, uint64(now.Unix()))
		vm.mem.WriteU64(tv+8, uint64(now.Nanosecond()/1000))
		a[10] = 0
	case 278: // getrandom
		buf := a[10]
		length := int(a[11])
		_ = a[12] // flags
		if buf == 0 {
			a[10] = 0
			return
		}
		var b [64]byte
		for length > 0 {
			n := len(b)
			if n > length {
				n = length
			}
			rand.Read(b[:n])
			vm.mem.Write(buf, b[:n])
			buf += uint64(n)
			length -= n
		}
		a[10] = uint64(int64(a[11]))
	case 258: // riscv_hwprobe
		pairs := a[10]
		count := int(a[11])
		for i := 0; i < count; i++ {
			// unsupported keys: set key = -1, value = 0
			vm.mem.WriteU64(pairs+uint64(i)*16, ^uint64(0))
			vm.mem.WriteU64(pairs+uint64(i)*16+8, 0)
		}
		a[10] = 0
	case 259: // riscv_flush_icache
		a[10] = 0
	case 167: // prctl
		a[10] = neg(errEINVAL)
	case 96: // set_tid_address
		vm.clearChildTid = a[10]
		vm.clearChildSet = true
		a[10] = uint64(vm.tid)
	case 261: // prlimit64
		pid := int64(a[10])
		resource := int64(a[11])
		newlim := a[12]
		oldlim := a[13]
		_ = pid
		if oldlim != 0 {
			var cur, max uint64
			switch resource {
			case 7: // RLIMIT_NOFILE
				cur, max = 1024, 1024
			default:
				cur, max = ^uint64(0), ^uint64(0) // infinity
			}
			vm.mem.WriteU64(oldlim, cur)
			vm.mem.WriteU64(oldlim+8, max)
		}
		if newlim != 0 {
			vm.mem.ReadU64(newlim)
		}
		a[10] = 0
	case 93: // exit
		vm.exited = true
		vm.state = stateExited
		s.m.exitCode = int(int32(a[10]))
	case 94: // exit_group
		s.m.exited = true
		s.m.exitCode = int(int32(a[10]))
		for _, t := range s.m.threads {
			t.exited = true
			t.state = stateExited
		}
	case 160: // uname
		a[10] = 0
	case 20: // epoll_create1
		a[10] = s.doEpollCreate(vm)
	case 19: // eventfd2
		a[10] = s.doEventfd2(vm, a[10], a[11])
	case 21: // epoll_ctl
		a[10] = uint64(s.doEpollCtl(vm, a[10], a[11], a[12], a[13]))
	case 22: // epoll_pwait
		a[10] = uint64(s.doEpollPwait(vm, a[10], a[11], a[12], a[13], a[14]))
	case 17: // getcwd
		a[10] = uint64(s.doGetcwd(vm, a[10], a[11]))
	case 48: // faccessat
		a[10] = 0
	case 49: // chdir
		a[10] = 0
	case 78: // readlinkat
		a[10] = neg(errEINVAL)
	case 79: // fstatat
		a[10] = uint64(s.doFstatat(vm, a[10], a[11], a[12]))
	case 80: // fstat
		a[10] = uint64(s.doFstat(vm, a[10], a[11]))
	case 59: // pipe2
		a[10] = uint64(s.doPipe2(vm, a[10], a[11]))
	case 29: // ioctl
		a[10] = uint64(s.doIoctl(vm, a[10], a[11], a[12]))
	case 198: // socket
		a[10] = s.doSocket(vm, a[10], a[11], a[12])
	case 200: // bind
		a[10] = s.doBind(vm, a[10], a[11], a[12])
	case 201: // listen
		a[10] = s.doListen(vm, a[10], a[11])
	case 202: // accept
		a[10] = s.doAccept(vm, a[10], a[11], a[12])
	case 242: // accept4
		a[10] = s.doAccept4(vm, a[10], a[11], a[12], a[13])
	case 203: // connect
		a[10] = s.doConnect(vm, a[10], a[11], a[12])
	case 204: // getsockname
		a[10] = s.doGetsockname(vm, a[10], a[11], a[12])
	case 205: // getpeername
		a[10] = s.doGetpeername(vm, a[10], a[11], a[12])
	case 206: // sendto
		a[10] = s.doSendto(vm, a[10], a[11], a[12], a[13], a[14], a[15])
	case 207: // recvfrom
		a[10] = s.doRecvfrom(vm, a[10], a[11], a[12], a[13], a[14], a[15])
	case 208: // setsockopt
		a[10] = s.doSetsockopt(vm, a[10], a[11], a[12], a[13], a[14])
	case 209: // getsockopt
		a[10] = s.doGetsockopt(vm, a[10], a[11], a[12], a[13], a[14])
	case 210: // shutdown
		a[10] = s.doShutdown(vm, a[10], a[11])
	case 211: // sendmsg
		a[10] = s.doSendmsg(vm, a[10], a[11], a[12])
	case 212: // recvmsg
		a[10] = s.doRecvmsg(vm, a[10], a[11], a[12])
	default:
		vm.faultf("unknown syscall %d", num)
	}
}

// helpers -------------------------------------------------------------------

func (m *Mem) ReadCString(a uint64) string {
	var b []byte
	for {
		c := m.ReadU8(a)
		if c == 0 {
			return string(b)
		}
		b = append(b, c)
		a++
		if len(b) > 4096 {
			return string(b)
		}
	}
}

func (vm *VM) faultf(format string, args ...interface{}) {
	panic(&Fault{Msg: sprintf(format, args...), PC: vm.m.prog.addr(vm.pc)})
}

func sprintf(format string, args ...interface{}) string {
	return fmtSprintf(format, args...)
}
