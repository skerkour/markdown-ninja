package riscvemu

// Socket and epoll syscalls. Guest sockets are bridged to the host network
// stack so the guest (including its pure-Go crypto/tls) can make real network
// connections (e.g. HTTPS fetch). The socket layer is kept behind the syscall
// boundary (Syscalls) so a future sandbox/Gofer can mediate it.

import (
	"errors"
	"net"
	"os"
	"strings"
	"syscall"
	"time"
)

// sock types
const (
	sockStream = 1 // SOCK_STREAM
	sockDgram  = 2 // SOCK_DGRAM
)

// sock models a guest socket bridged to the host.
type sock struct {
	fd       int
	socktype int
	family   int

	conn     net.Conn       // connected stream / connected UDP
	listener net.Listener   // listening stream
	pc       net.PacketConn // unconnected UDP

	hostFD int // raw host fd for readiness; -1 if none

	peekBuf   []byte // data read during readiness probes
	errorSeen bool   // terminal read error already surfaced to the guest
}

func (s *Syscalls) newSock(socktype, family int) int {
	fd := s.allocFDNum()
	s.socks[fd] = &sock{fd: fd, socktype: socktype, family: family, hostFD: -1}
	return fd
}

func (s *Syscalls) allocFDNum() int {
	fd := s.nextFD
	s.nextFD++
	return fd
}

func (s *Syscalls) getSock(fd int) *sock {
	return s.socks[fd]
}

// initHostFd extracts and caches the raw host fd of a connection, marking it
// non-blocking. Returns -1 if no raw fd is available.
func (sk *sock) initHostFd() int {
	if sk.hostFD >= 0 {
		return sk.hostFD
	}
	var c interface {
		SyscallConn() (syscall.RawConn, error)
	}
	switch {
	case sk.conn != nil:
		c, _ = sk.conn.(interface {
			SyscallConn() (syscall.RawConn, error)
		})
	case sk.pc != nil:
		c, _ = sk.pc.(interface {
			SyscallConn() (syscall.RawConn, error)
		})
	default:
		return -1
	}
	if c == nil {
		return -1
	}
	raw, err := c.SyscallConn()
	if err != nil {
		return -1
	}
	var fd uintptr
	raw.Control(func(f uintptr) { fd = f })
	if fd == 0 {
		return -1
	}
	sk.hostFD = int(fd)
	return sk.hostFD
}

// pipePair models an eventfd as a pipe (read = eventfd fd).
type pipePair struct {
	read  *os.File
	write *os.File
}

// ready reports whether the socket has readable data, an error, or EOF.
// It uses select on the raw host fd, which is what the socket poller uses.
func (sk *sock) ready() bool {
	if len(sk.peekBuf) > 0 {
		return true
	}
	if sk.conn == nil {
		return false
	}
	if sk.hostFD < 0 {
		sk.initHostFd()
	}
	if sk.hostFD < 0 {
		return false
	}
	rfds := &syscall.FdSet{}
	rfds.Bits[sk.hostFD/64] |= 1 << (uint(sk.hostFD) % 64)
	for {
		n, err := syscall.Select(sk.hostFD+1, rfds, nil, nil, &syscall.Timeval{})
		if err == syscall.EINTR {
			continue
		}
		return err == nil && n > 0
	}
}

// readFromHost reads up to n bytes, parking the guest thread until data is
// available or the host reports an error/EOF. Returns (n, errno) where errno
// is 0 on success.
func (sk *sock) readFromHost(vm *VM, buf []byte) (int, int) {
	if sk.conn == nil {
		return 0, int(errEBADF)
	}
	if len(sk.peekBuf) > 0 {
		n := copy(buf, sk.peekBuf)
		sk.peekBuf = sk.peekBuf[n:]
		return n, 0
	}
	for {
		// generous deadline: the guest only reads after epoll reported the fd
		// ready, so data should be present.
		sk.conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		n, err := sk.conn.Read(buf)
		if n > 0 {
			return n, 0
		}
		if err != nil {
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				// no data yet; park briefly and retry so host I/O can arrive
				vm.wakeTime = time.Now().Add(2 * time.Millisecond)
				vm.state = stateBlockedSleep
				return 0, int(errEAGAIN)
			}
			if err == os.ErrDeadlineExceeded {
				vm.wakeTime = time.Now().Add(2 * time.Millisecond)
				vm.state = stateBlockedSleep
				return 0, int(errEAGAIN)
			}
			// terminal error (EOF/reset): surface it to the guest and close the
			// socket so it stops being polled.
			sk.conn.Close()
			sk.conn = nil
			sk.hostFD = -1
			return 0, int(errEINVAL)
		}
	}
}

// --- epoll -------------------------------------------------------------------

func (s *Syscalls) doEpollCreate(vm *VM) uint64 {
	fd := s.allocFDNum()
	s.epollfds[fd] = struct{}{}
	s.epollReg[fd] = make(map[uint64]epollReg)
	return uint64(fd)
}

func (s *Syscalls) doEpollCtl(vm *VM, epfd, op, fd, event uint64) uint64 {
	regs, ok := s.epollReg[int(epfd)]
	if !ok {
		return neg(errEBADF)
	}
	switch int64(op) {
	case 1: // EPOLL_CTL_ADD
		regs[fd] = epollReg{
			fd:   fd,
			data: vm.mem.ReadU64(event + 8), // EpollEvent.Data at offset 8
			ev:   vm.mem.ReadU32(event),
		}
	case 2: // EPOLL_CTL_MOD
		if r, ok2 := regs[fd]; ok2 {
			r.data = vm.mem.ReadU64(event + 8)
			r.ev = vm.mem.ReadU32(event)
			regs[fd] = r
		}
	case 3: // EPOLL_CTL_DEL
		delete(regs, fd)
	default:
		return neg(errEINVAL)
	}
	return 0
}

func (s *Syscalls) doEpollPwait(vm *VM, epfd, events, maxevents, timeout, sigmask uint64) uint64 {
	_ = epfd
	_ = sigmask
	var ready []epollReg
	regs := s.epollReg[int(epfd)]
	for _, r := range regs {
		if sk := s.socks[int(r.fd)]; sk != nil {
			if sk.ready() {
				ready = append(ready, r)
			}
		}
	}
	n := int64(len(ready))
	if int(maxevents) < len(ready) {
		n = int64(maxevents)
	}
	if os.Getenv("RISCVEMU_TRACESYS") != "" {
		rl := ""
		for i := int64(0); i < n; i++ {
			rl += sprintf(" %d", ready[i].fd)
		}
		println("EPOLL-RET t", vm.tid, "n", n, "regs", len(regs), "ready", rl, "tmo", timeout)
	}
	if n == 0 {
		// nothing ready yet: park the thread and arm a host-side poller that
		// wakes the machine when a registered socket becomes ready.
		s.armSocketPoller(vm, regs)
		if timeout != 0 && timeout != ^uint64(0) {
			vm.wakeTime = time.Now().Add(time.Millisecond * time.Duration(int64(timeout)))
		} else {
			vm.wakeTime = time.Now().Add(50 * time.Millisecond)
		}
		vm.state = stateBlockedSleep
		return 0
	}
	for i := int64(0); i < n; i++ {
		// EpollEvent: events @0, pad @4, data @8, size 16
		vm.mem.WriteU32(events+uint64(i)*16, ready[i].ev|0x1) // EPOLLIN
		vm.mem.WriteU64(events+uint64(i)*16+8, ready[i].data)
	}
	return uint64(n)
}

// armSocketPoller spawns a background goroutine that polls the registered host
// fds and signals the machine's netWake channel when one becomes readable, so
// a thread parked in epoll_pwait is woken promptly by host I/O.
func (s *Syscalls) armSocketPoller(vm *VM, regs map[uint64]epollReg) {
	m := s.m
	m.netPollers++
	go func() {
		defer func() { m.netPollers-- }()
		for {
			// collect host fds to watch
			var fds []int
			for _, r := range regs {
				if sk := s.socks[int(r.fd)]; sk != nil {
					fd := sk.hostFD
					if fd < 0 {
						fd = sk.initHostFd()
					}
					if fd >= 0 {
						fds = append(fds, fd)
					}
				}
			}
			if len(fds) == 0 {
				time.Sleep(10 * time.Millisecond)
				continue
			}
			rfds := &syscall.FdSet{}
			maxfd := 0
			for _, fd := range fds {
				rfds.Bits[fd/64] |= 1 << (uint(fd) % 64)
				if fd > maxfd {
					maxfd = fd
				}
			}
			_, err := syscall.Select(maxfd+1, rfds, nil, nil, &syscall.Timeval{Sec: 0, Usec: 10000})
			if err == nil {
				if os.Getenv("RISCVEMU_TRACESYS") != "" {
					println("SOCKET-POLLER fired, fds", len(fds), "maxfd", maxfd)
				}
				select {
				case m.netWake <- struct{}{}:
				default:
				}
				return
			}
		}
	}()
}

func fdReady(fd int) bool {
	if fd < 0 {
		return false
	}
	rfds := &syscall.FdSet{}
	rfds.Bits[fd/64] |= 1 << (uint(fd) % 64)
	for {
		n, err := syscall.Select(fd+1, rfds, nil, nil, &syscall.Timeval{})
		if err == syscall.EINTR {
			continue
		}
		return err == nil && n > 0
	}
}

// doEventfd2 creates an eventfd (modelled as a pipe) and returns its guest fd.
func (s *Syscalls) doEventfd2(vm *VM, initval, flags uint64) uint64 {
	_ = initval
	_ = flags
	r, w, err := os.Pipe()
	if err != nil {
		return neg(errENOMEM)
	}
	gfd := s.allocFDNum()
	s.eventfds[gfd] = &pipePair{read: r, write: w}
	return uint64(gfd)
}

// --- socket syscalls ----------------------------------------------------------

func parseSockaddrInet(b []byte) (ip net.IP, port int, family int) {
	if len(b) < 8 {
		return nil, 0, 0
	}
	family = int(byteOrderU16(b[0:2]))
	switch family {
	case 2: // AF_INET
		port = int(byteOrderU16be(b[2:4]))
		ip = net.IPv4(b[4], b[5], b[6], b[7])
	case 10, 28: // AF_INET6
		if len(b) < 28 {
			return nil, 0, family
		}
		port = int(byteOrderU16be(b[2:4]))
		ip = make(net.IP, 16)
		copy(ip, b[8:24])
	}
	return ip, port, family
}

func (s *Syscalls) doSocket(vm *VM, domain, socktype, proto uint64) uint64 {
	_ = vm
	st := int(socktype & 0x7f)
	if st != sockStream && st != sockDgram {
		st = sockStream
	}
	fd := s.newSock(st, int(domain))
	return uint64(fd)
}

func (s *Syscalls) doConnect(vm *VM, fd, addr, addrlen uint64) uint64 {
	sk := s.getSock(int(fd))
	if sk == nil {
		return neg(errEBADF)
	}
	data := vm.mem.ReadBytes(addr, int(addrlen))
	ip, port, _ := parseSockaddrInet(data)
	if ip == nil {
		return neg(errEINVAL)
	}
	target := &net.TCPAddr{IP: ip, Port: port}
	if os.Getenv("RISCVEMU_TRACESYS") != "" {
		println("connect fd", fd, "->", target.String())
	}
	var err error
	if sk.socktype == sockStream {
		d := net.Dialer{Timeout: 10 * time.Second}
		var conn net.Conn
		conn, err = d.Dial("tcp", target.String())
		if err == nil {
			sk.conn = conn
			sk.initHostFd()
		}
	} else {
		var c *net.UDPConn
		c, err = net.DialUDP("udp", nil, &net.UDPAddr{IP: ip, Port: port})
		if err == nil {
			sk.conn = c
			sk.initHostFd()
		}
	}
	if err != nil {
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			return neg(errETIMEDOUT)
		}
		return neg(errECONNREFUSED)
	}
	return 0
}

func (s *Syscalls) doBind(vm *VM, fd, addr, addrlen uint64) uint64 {
	_ = vm
	if s.getSock(int(fd)) == nil {
		return neg(errEBADF)
	}
	return neg(errEADDRINUSE)
}

func (s *Syscalls) doListen(vm *VM, fd, backlog uint64) uint64 {
	sk := s.getSock(int(fd))
	if sk == nil || sk.conn == nil {
		return neg(errEBADF)
	}
	laddr := sk.conn.LocalAddr()
	tcp, ok := laddr.(*net.TCPAddr)
	if !ok {
		return neg(errEINVAL)
	}
	sk.conn.Close()
	l, err := net.Listen("tcp", tcp.String())
	if err != nil {
		return neg(errEADDRINUSE)
	}
	sk.listener = l
	sk.conn = nil
	sk.hostFD = -1
	if tl, ok2 := l.(*net.TCPListener); ok2 {
		if rc, err := tl.SyscallConn(); err == nil {
			rc.Control(func(f uintptr) {
				sk.hostFD = int(f)
			})
		}
	}
	return 0
}

func (s *Syscalls) doAccept(vm *VM, fd, addr, addrlen uint64) uint64 {
	sk := s.getSock(int(fd))
	if sk == nil || sk.listener == nil {
		return neg(errEBADF)
	}
	conn, err := sk.listener.Accept()
	if err != nil {
		return neg(errEAGAIN)
	}
	nfd := s.newSock(sockStream, 2)
	nsk := s.socks[nfd]
	nsk.conn = conn
	nsk.initHostFd()
	if addr != 0 {
		writeInetAddr(vm, addr, conn.RemoteAddr())
	}
	return uint64(nfd)
}

func (s *Syscalls) doAccept4(vm *VM, fd, addr, addrlen, flags uint64) uint64 {
	_ = flags
	return s.doAccept(vm, fd, addr, addrlen)
}

func writeInetAddr(vm *VM, addr uint64, ra net.Addr) {
	tcp, ok := ra.(*net.TCPAddr)
	if !ok {
		return
	}
	if ip := tcp.IP.To4(); ip != nil {
		byteOrderPutU16be(vm.mem, addr, 2)
		byteOrderPutU16be(vm.mem, addr+2, uint16(tcp.Port))
		b := vm.mem.ReadBytes(addr+4, 4)
		copy(b, ip)
		vm.mem.Write(addr+4, b)
	} else if ip16 := tcp.IP.To16(); ip16 != nil {
		byteOrderPutU16be(vm.mem, addr, 28)
		byteOrderPutU16be(vm.mem, addr+2, uint16(tcp.Port))
		b := vm.mem.ReadBytes(addr+8, 16)
		copy(b, ip16)
		vm.mem.Write(addr+8, b)
	}
}

func (s *Syscalls) doSendto(vm *VM, fd, buf, n, flags, dst, addrlen uint64) uint64 {
	sk := s.getSock(int(fd))
	if sk == nil {
		return neg(errEBADF)
	}
	b := vm.mem.ReadBytes(buf, int(n))
	_ = flags
	if sk.conn != nil {
		w, err := sk.conn.Write(b)
		if err != nil {
			return neg(errEPIPE)
		}
		return uint64(w)
	}
	return neg(errECONNREFUSED)
}

func (s *Syscalls) doRecvfrom(vm *VM, fd, buf, n, flags, src, addrlen uint64) uint64 {
	sk := s.getSock(int(fd))
	if sk == nil {
		return neg(errEBADF)
	}
	out := make([]byte, int(n))
	r, errno := sk.readFromHost(vm, out)
	if errno != 0 {
		return neg(int64(errno))
	}
	vm.mem.Write(buf, out[:r])
	if src != 0 && sk.conn != nil {
		writeInetAddr(vm, src, sk.conn.RemoteAddr())
	}
	return uint64(r)
}

func (s *Syscalls) doShutdown(vm *VM, fd, how uint64) uint64 {
	sk := s.getSock(int(fd))
	if sk == nil {
		return neg(errEBADF)
	}
	if sk.conn != nil {
		sk.conn.Close()
	}
	return 0
}

func (s *Syscalls) doGetsockname(vm *VM, fd, addr, addrlen uint64) uint64 {
	sk := s.getSock(int(fd))
	if sk == nil || sk.conn == nil {
		return neg(errEBADF)
	}
	writeInetAddr(vm, addr, sk.conn.LocalAddr())
	vm.mem.WriteU32(addrlen, 16)
	return 0
}

func (s *Syscalls) doGetpeername(vm *VM, fd, addr, addrlen uint64) uint64 {
	sk := s.getSock(int(fd))
	if sk == nil || sk.conn == nil {
		return neg(errEBADF)
	}
	writeInetAddr(vm, addr, sk.conn.RemoteAddr())
	vm.mem.WriteU32(addrlen, 16)
	return 0
}

func (s *Syscalls) doSetsockopt(vm *VM, fd, level, opt, val, vallen uint64) uint64 {
	_ = vm
	_ = fd
	_ = level
	_ = opt
	_ = val
	_ = vallen
	return 0
}

func (s *Syscalls) doGetsockopt(vm *VM, fd, level, opt, val, vallen uint64) uint64 {
	if opt == 4 { // SO_ERROR
		vm.mem.WriteU32(val, 0)
	}
	_ = vallen
	return 0
}

func (s *Syscalls) doSendmsg(vm *VM, fd, msg, flags uint64) uint64 {
	sk := s.getSock(int(fd))
	if sk == nil {
		return neg(errEBADF)
	}
	iov := vm.mem.ReadU64(msg + 24)
	iovcnt := int(vm.mem.ReadU64(msg + 32))
	var all []byte
	for i := 0; i < iovcnt; i++ {
		base := vm.mem.ReadU64(iov + uint64(i)*16)
		ln := int(vm.mem.ReadU64(iov + uint64(i)*16 + 8))
		all = append(all, vm.mem.ReadBytes(base, ln)...)
	}
	if sk.conn != nil {
		w, err := sk.conn.Write(all)
		if err != nil {
			return neg(errEPIPE)
		}
		return uint64(w)
	}
	return neg(errECONNREFUSED)
}

func (s *Syscalls) doRecvmsg(vm *VM, fd, msg, flags uint64) uint64 {
	sk := s.getSock(int(fd))
	if sk == nil {
		return neg(errEBADF)
	}
	iov := vm.mem.ReadU64(msg + 24)
	iovcnt := int(vm.mem.ReadU64(msg + 32))
	total := 0
	for i := 0; i < iovcnt; i++ {
		total += int(vm.mem.ReadU64(iov + uint64(i)*16 + 8))
	}
	buf := make([]byte, total)
	r, errno := sk.readFromHost(vm, buf)
	if errno != 0 {
		return neg(int64(errno))
	}
	off := 0
	for i := 0; i < iovcnt && off < r; i++ {
		base := vm.mem.ReadU64(iov + uint64(i)*16)
		ln := int(vm.mem.ReadU64(iov + uint64(i)*16 + 8))
		n := r - off
		if n > ln {
			n = ln
		}
		vm.mem.Write(base, buf[off:off+n])
		off += n
	}
	return uint64(r)
}

// --- misc --------------------------------------------------------------------

func (s *Syscalls) doGetcwd(vm *VM, buf, size uint64) uint64 {
	if size == 0 {
		return neg(errEINVAL)
	}
	vm.mem.WriteU8(buf, '/')
	if size > 1 {
		vm.mem.WriteU8(buf+1, 0)
	}
	return 1
}

func (s *Syscalls) doFstatat(vm *VM, dirfd, path, st uint64) uint64 {
	return s.writeFstat(vm, st)
}

func (s *Syscalls) doFstat(vm *VM, fd, st uint64) uint64 {
	return s.writeFstat(vm, st)
}

// writeFstat writes a synthetic stat struct so the runtime sees regular files.
func (s *Syscalls) writeFstat(vm *VM, st uint64) uint64 {
	vm.mem.WriteU64(st, 0)         // st_dev
	vm.mem.WriteU64(st+8, 1)       // st_ino
	vm.mem.WriteU32(st+16, 0x81A4) // st_mode: regular file, 0644
	vm.mem.WriteU64(st+24, 1)      // st_nlink
	vm.mem.WriteU32(st+28, 0)      // st_uid
	vm.mem.WriteU32(st+32, 0)      // st_gid
	vm.mem.WriteU64(st+40, 0)      // st_rdev
	vm.mem.WriteU64(st+48, 0)      // st_size
	vm.mem.WriteU64(st+56, 4096)   // st_blksize
	return 0
}

func (s *Syscalls) doPipe2(vm *VM, pipefd, flags uint64) uint64 {
	_ = flags
	r, w, err := os.Pipe()
	if err != nil {
		return neg(errENOMEM)
	}
	rfd := s.allocFDNum()
	wfd := s.allocFDNum()
	s.pipes[rfd] = r
	s.pipes[wfd] = w
	vm.mem.WriteU32(pipefd, uint32(rfd))
	vm.mem.WriteU32(pipefd+4, uint32(wfd))
	return 0
}

func (s *Syscalls) doIoctl(vm *VM, fd, req, arg uint64) uint64 {
	_ = vm
	_ = fd
	_ = req
	_ = arg
	return 0
}

// --- byte-order helpers (little-endian, riscv64) -------------------------------

// hexStr renders b as hex for debugging.
func hexStr(b []byte) string {
	const hex = "0123456789abcdef"
	out := make([]byte, 0, len(b)*3)
	for i, c := range b {
		if i > 0 {
			out = append(out, ' ')
		}
		out = append(out, hex[c>>4], hex[c&0xf])
	}
	return string(out)
}

func byteOrderU16(b []byte) uint16 { return uint16(b[0]) | uint16(b[1])<<8 }

// byteOrderU16 reads a little-endian uint16.

func byteOrderU16be(b []byte) uint16 { return uint16(b[0])<<8 | uint16(b[1]) }

func byteOrderPutU16(mem *Mem, a uint64, v uint16) {
	mem.WriteU8(a, byte(v))
	mem.WriteU8(a+1, byte(v>>8))
}

func byteOrderPutU16be(mem *Mem, a uint64, v uint16) {
	mem.WriteU8(a, byte(v>>8))
	mem.WriteU8(a+1, byte(v))
}

// nameserver returns the DNS nameserver to present to the guest, derived from
// the host's resolver configuration.
func nameserver() string {
	if v := os.Getenv("RISCVEMU_NAMESERVER"); v != "" {
		return v
	}
	b, err := os.ReadFile("/etc/resolv.conf")
	if err != nil {
		return "8.8.8.8"
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "nameserver") {
			f := strings.Fields(line)
			if len(f) >= 2 {
				return f[1]
			}
		}
	}
	return "8.8.8.8"
}

// resolvConf returns the guest /etc/resolv.conf contents.
func (s *Syscalls) resolvConf() string {
	return "nameserver " + nameserver() + "\noptions ndots:0\n"
}

// hostsFile returns a minimal guest /etc/hosts.
func (s *Syscalls) hostsFile() string {
	return "127.0.0.1 localhost\n"
}
