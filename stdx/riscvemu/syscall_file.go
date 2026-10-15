package riscvemu

import (
	"crypto/rand"
	"fmt"
	"os"
	"strings"
	"time"
)

func fmtSprintf(format string, args ...interface{}) string {
	return fmt.Sprintf(format, args...)
}

func crandRead(b []byte) {
	rand.Read(b)
}

func cryptoRand(b []byte) {
	rand.Read(b)
}

// --- errno helpers ----------------------------------------------------------

const (
	errEAGAIN       = -11
	errENOENT       = -2
	errEINVAL       = -22
	errEBADF        = -9
	errENOMEM       = -12
	errETIMEDOUT    = -110
	errECONNREFUSED = -111
	errEPIPE        = -32
	errEADDRINUSE   = -98
	errEAFNOSUPPORT = -97
)

func neg(n int64) uint64 { return uint64(n) }

// --- file operations --------------------------------------------------------

func (s *Syscalls) doOpen(vm *VM, name string) int {
	// Virtual filesystem: serve the paths the Go runtime probes at startup.
	switch {
	case name == "/dev/urandom" || name == "/dev/random":
		return s.allocFd(&vfile{data: nil, name: name, dynamic: dynamicRandom})
	case strings.HasPrefix(name, "/proc/self/auxv"):
		// Provide a real auxv so the runtime sees random/cpu info.
		return s.allocFd(&vfile{data: nil, name: name, dynamic: dynamicAuxv})
	case strings.HasPrefix(name, "/proc/self/maps"):
		return s.allocFd(&vfile{data: nil, name: name, dynamic: dynamicEmpty})
	case name == "/dev/null":
		return s.allocFd(&vfile{data: nil, name: name})
	case name == "/etc/resolv.conf":
		return s.allocFd(&vfile{data: []byte(s.resolvConf()), name: name})
	case name == "/etc/hosts":
		return s.allocFd(&vfile{data: []byte(s.hostsFile()), name: name})
	case name == "/etc/nsswitch.conf":
		return s.allocFd(&vfile{data: []byte("hosts: files dns\n"), name: name})
	case name == "/etc/services":
		return s.allocFd(&vfile{data: []byte("http 80/tcp\nhttps 443/tcp\n"), name: name})
	case strings.HasPrefix(name, "/sys/kernel/mm/transparent_hugepage/hpage_pmd_size"):
		return s.allocFd(&vfile{data: []byte("2097152\n"), name: name})
	case name == "/etc/ssl/certs/ca-certificates.crt" ||
		name == "/etc/pki/tls/certs/ca-bundle.crt" ||
		name == "/etc/ssl/cert.pem":
		data, _ := os.ReadFile("/etc/ssl/certs/ca-certificates.crt")
		if len(data) == 0 {
			data = []byte("")
		}
		return s.allocFd(&vfile{data: data, name: name})
	default:
		return int(errENOENT)
	}
}

const (
	dynamicNone = iota
	dynamicRandom
	dynamicAuxv
	dynamicEmpty
)

func (s *Syscalls) allocFd(f *vfile) int {
	f.fd = s.nextFD
	s.fds[f.fd] = f
	s.nextFD++
	return f.fd
}

func (s *Syscalls) doClose(fd int) {
	if f, ok := s.fds[fd]; ok {
		delete(s.fds, fd)
		_ = f
	}
	if sk, ok := s.socks[fd]; ok {
		if sk.conn != nil {
			sk.conn.Close()
		}
		if sk.listener != nil {
			sk.listener.Close()
		}
		delete(s.socks, fd)
	}
	if p, ok := s.pipes[fd]; ok {
		p.Close()
		delete(s.pipes, fd)
	}
	if ef, ok := s.eventfds[fd]; ok {
		ef.read.Close()
		ef.write.Close()
		delete(s.eventfds, fd)
	}
	delete(s.epollfds, fd)
}

func (s *Syscalls) doRead(vm *VM, fd int, buf uint64, n int) int {
	switch fd {
	case 0: // stdin
		if n <= 0 {
			return 0
		}
		var b [1024]byte
		if n > len(b) {
			n = len(b)
		}
		r, err := s.stdin.Read(b[:n])
		if err != nil && r == 0 {
			return 0 // EOF
		}
		vm.mem.Write(buf, b[:r])
		return r
	}
	if sk := s.socks[fd]; sk != nil {
		out := make([]byte, n)
		r, errno := sk.readFromHost(vm, out)
		if os.Getenv("RISCVEMU_TRACESYS") != "" {
			println("sockRead fd", fd, "n", n, "got", r, "errno", errno)
		}
		if errno != 0 {
			return errno
		}
		vm.mem.Write(buf, out[:r])
		return r
	}
	if p := s.pipes[fd]; p != nil {
		b := make([]byte, n)
		r, err := p.Read(b)
		if err != nil && r == 0 {
			return 0
		}
		vm.mem.Write(buf, b[:r])
		return r
	}
	if ef := s.eventfds[fd]; ef != nil {
		b := make([]byte, n)
		r, err := ef.read.Read(b)
		if err != nil && r == 0 {
			return 0
		}
		vm.mem.Write(buf, b[:r])
		return r
	}
	f, ok := s.fds[fd]
	if !ok {
		return int(errENOENT)
	}
	return f.read(vm, buf, n)
}

func (s *Syscalls) doWrite(vm *VM, fd int, buf uint64, n int) int {
	switch fd {
	case 1:
		b := vm.mem.ReadBytes(buf, n)
		w, err := s.stdout.Write(b)
		if err != nil {
			return 0
		}
		return w
	case 2:
		b := vm.mem.ReadBytes(buf, n)
		w, err := s.stderr.Write(b)
		if err != nil {
			return 0
		}
		return w
	}
	if sk := s.socks[fd]; sk != nil {
		if sk.conn == nil {
			return int(errEPIPE)
		}
		b := vm.mem.ReadBytes(buf, n)
		w, err := sk.conn.Write(b)
		if os.Getenv("RISCVEMU_TRACESYS") != "" {
			if sk.socktype == sockStream && len(b) > 100 {
				os.WriteFile("/tmp/riscvemu_clienthello.bin", b, 0644)
				os.WriteFile("/tmp/riscvemu_remote.txt", []byte(sk.conn.RemoteAddr().String()), 0644)
				println("sockWrite fd", fd, "n", len(b), "to", sk.conn.RemoteAddr().String())
			} else {
				println("sockWrite fd", fd, "n", len(b), "w", w, "err", err != nil)
			}
		}
		if err != nil {
			return int(errEPIPE)
		}
		return w
	}
	if p := s.pipes[fd]; p != nil {
		b := vm.mem.ReadBytes(buf, n)
		w, err := p.Write(b)
		if err != nil {
			return int(errEPIPE)
		}
		return w
	}
	if ef := s.eventfds[fd]; ef != nil {
		b := vm.mem.ReadBytes(buf, n)
		w, err := ef.write.Write(b)
		if err != nil {
			return int(errEPIPE)
		}
		return w
	}
	f, ok := s.fds[fd]
	if !ok {
		return int(errENOENT)
	}
	return f.write(vm, buf, n)
}

func (s *Syscalls) doPread(vm *VM, fd int, buf uint64, n int, off int64) int {
	f, ok := s.fds[fd]
	if !ok {
		return int(errENOENT)
	}
	return f.pread(vm, buf, n, off)
}

func (s *Syscalls) doLseek(fd int, off int64, whence int) int {
	f, ok := s.fds[fd]
	if !ok {
		return int(errENOENT)
	}
	switch whence {
	case 0: // SEEK_SET
		if off < 0 {
			return int(errEINVAL)
		}
		f.pos = int(off)
	case 1: // SEEK_CUR
		f.pos += int(off)
	case 2: // SEEK_END
		f.pos = len(f.data) + int(off)
	}
	if f.pos < 0 {
		f.pos = 0
	}
	return f.pos
}

func (s *Syscalls) doBrk(addr uint64) uint64 {
	// Go calls brk(0) to query; return a fixed heap region.
	if s.breakAddr == 0 {
		s.breakAddr = s.m.stackStart - 64<<20
	}
	if addr == 0 {
		return s.breakAddr
	}
	if addr > s.breakAddr {
		s.breakAddr = addr
	}
	return s.breakAddr
}

// --- mmap -------------------------------------------------------------------

type region struct {
	start, size uint64
}

func (m *Machine) mmap(vm *VM, addr, length, prot, flags uint64, fd int, off uint64) uint64 {
	if length == 0 {
		return neg(errEINVAL)
	}
	length = (length + PageSize - 1) &^ uint64(PageSize-1)

	// anonymous only; file-backed maps not needed for hello-world
	fixed := flags&0x10 != 0 // MAP_FIXED
	if !fixed && addr == 0 {
		// allocate downward from the mmap cursor
		addr = m.mmapCursor - length
		addr &^= uint64(PageSize - 1)
		m.mmapCursor = addr
	} else {
		addr &^= uint64(PageSize - 1)
	}
	m.regions = append(m.regions, region{start: addr, size: length})
	return addr
}

func (m *Machine) munmap(addr, length uint64) {
	m.mem.DropPageRange(addr, length)
}

func (m *Machine) mprotect(addr, length, prot uint64) {
	// pages are fault-free; ignore protection
}

func (m *Machine) mincore(addr, length, vec uint64) {
	// report all present
	n := (length + PageSize - 1) / PageSize
	for i := uint64(0); i < n; i++ {
		m.mem.WriteU8(vec+i, 1)
	}
}

// --- clone / futex ----------------------------------------------------------

func (s *Syscalls) doClone(vm *VM, flags, stk uint64) {
	child := &VM{
		m:     s.m,
		mem:   s.m.mem,
		x:     vm.x,
		f:     vm.f,
		fcsr:  vm.fcsr,
		pc:    vm.pc, // parent's pc has already advanced past the ecall
		state: stateRunnable,
		tid:   s.m.nextTid,
	}
	s.m.nextTid++
	child.x[2] = stk // sp
	child.x[10] = 0  // a0 = 0 in child
	s.m.threads = append(s.m.threads, child)

	// parent gets the new tid
	vm.x[10] = uint64(child.tid)
}

func (s *Syscalls) doFutex(vm *VM, addr uint64, op int64, val uint32, timeout uint64) {
	// mask off private flag and clock bit
	cmd := int(op & 0x7f)
	switch cmd {
	case 0: // FUTEX_WAIT
		cur := uint32(vm.mem.ReadU32(addr))
		if cur != val {
			vm.x[10] = neg(errEAGAIN)
			return
		}
		// park; honor an optional timeout so a timed wait can time out
		var deadline time.Time
		if timeout != 0 {
			sec := int64(vm.mem.ReadU64(timeout))
			nsec := int64(vm.mem.ReadU64(timeout + 8))
			deadline = time.Now().Add(time.Duration(sec)*time.Second + time.Duration(nsec))
		}
		vm.futexWaitUntil(addr, val, deadline)
		vm.x[10] = neg(errEAGAIN) // overridden by futexWake/advanceTime
	case 1: // FUTEX_WAKE
		n := int(val)
		woken := s.m.futexWake(addr, n)
		vm.x[10] = uint64(woken)
	case 2: // FUTEX_FD
		vm.x[10] = neg(errENOENT)
	default:
		vm.x[10] = neg(errEINVAL)
	}
}

// --- vfile dynamic content --------------------------------------------------

type vfile struct {
	data    []byte
	pos     int
	fd      int
	name    string
	dynamic int
}

func (f *vfile) content(s *Syscalls) []byte {
	switch f.dynamic {
	case dynamicRandom:
		b := make([]byte, 256)
		cryptoRand(b)
		return b
	case dynamicAuxv:
		return s.m.auxvBytes
	case dynamicEmpty:
		return nil
	}
	return f.data
}

func (f *vfile) read(vm *VM, buf uint64, n int) int {
	data := f.data
	// static data consumed as-is; dynamic regenerated on first read
	if f.dynamic != dynamicNone {
		data = f.content(vm.m.sys)
		f.data = data
		f.dynamic = dynamicNone
	}
	if f.pos >= len(data) {
		return 0
	}
	avail := len(data) - f.pos
	if n > avail {
		n = avail
	}
	vm.mem.Write(buf, data[f.pos:f.pos+n])
	f.pos += n
	return n
}

func (f *vfile) pread(vm *VM, buf uint64, n int, off int64) int {
	data := f.data
	if f.dynamic != dynamicNone {
		data = f.content(vm.m.sys)
		f.data = data
		f.dynamic = dynamicNone
	}
	if off >= int64(len(data)) {
		return 0
	}
	avail := len(data) - int(off)
	if n > avail {
		n = avail
	}
	vm.mem.Write(buf, data[int(off):int(off)+n])
	return n
}

func (f *vfile) write(vm *VM, buf uint64, n int) int {
	b := vm.mem.ReadBytes(buf, n)
	f.data = append(f.data, b...)
	return n
}
