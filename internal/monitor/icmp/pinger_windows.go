//go:build windows

package icmp

import (
	"context"
	"encoding/binary"
	"net/netip"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	iphlpapi          = windows.NewLazySystemDLL("iphlpapi.dll")
	procIcmpCreate    = iphlpapi.NewProc("IcmpCreateFile")
	procIcmp6Create   = iphlpapi.NewProc("Icmp6CreateFile")
	procIcmpClose     = iphlpapi.NewProc("IcmpCloseHandle")
	procIcmpSendEcho  = iphlpapi.NewProc("IcmpSendEcho")
	procIcmp6SendEcho = iphlpapi.NewProc("Icmp6SendEcho2")
)

type winPinger struct{}

// New returns the Windows pinger (IcmpSendEcho; no administrator rights needed).
func New() (Pinger, error) {
	if err := procIcmpSendEcho.Find(); err != nil {
		return unavailable{}, nil
	}
	return winPinger{}, nil
}

func (winPinger) Mode() string { return "windows" }

var payload = []byte("NetWatch-ICMP-probe-32-bytes....")

func (winPinger) Ping(ctx context.Context, addr netip.Addr, packets int, timeout time.Duration) Result {
	addr = addr.Unmap()
	create := procIcmpCreate
	if addr.Is6() {
		create = procIcmp6Create
	}
	h, _, err := create.Call()
	if windows.Handle(h) == windows.InvalidHandle {
		if err == windows.ERROR_ACCESS_DENIED {
			return Result{Sent: packets, Code: Permission}
		}
		return Result{Sent: packets, Code: Internal}
	}
	defer procIcmpClose.Call(h)
	return run(ctx, packets, timeout, func(_ context.Context, _ int, timeout time.Duration) (time.Duration, Code) {
		if addr.Is6() {
			return echo6(h, addr, timeout)
		}
		return echo4(h, addr, timeout)
	})
}

func echo4(h uintptr, addr netip.Addr, timeout time.Duration) (time.Duration, Code) {
	a4 := addr.As4()
	dest := *(*uint32)(unsafe.Pointer(&a4[0]))
	reply := make([]byte, 64+len(payload)+8)
	start := time.Now()
	n, _, err := procIcmpSendEcho.Call(h, uintptr(dest), uintptr(unsafe.Pointer(&payload[0])), uintptr(len(payload)),
		0, uintptr(unsafe.Pointer(&reply[0])), uintptr(len(reply)), uintptr(timeout.Milliseconds()))
	elapsed := time.Since(start)
	if n == 0 {
		return 0, classifyStatus(errno(err))
	}
	// ICMP_ECHO_REPLY: Address(4) Status(4) RoundTripTime(4) ...
	status := binary.LittleEndian.Uint32(reply[4:8])
	if code := classifyStatus(status); code != OK {
		return 0, code
	}
	if rtt := binary.LittleEndian.Uint32(reply[8:12]); rtt > 0 {
		return time.Duration(rtt) * time.Millisecond, OK
	}
	return elapsed, OK
}

// sockaddrIn6 mirrors SOCKADDR_IN6.
type sockaddrIn6 struct {
	Family   uint16
	Port     uint16
	Flowinfo uint32
	Addr     [16]byte
	ScopeID  uint32
}

func echo6(h uintptr, addr netip.Addr, timeout time.Duration) (time.Duration, Code) {
	src := sockaddrIn6{Family: windows.AF_INET6}
	dst := sockaddrIn6{Family: windows.AF_INET6, Addr: addr.As16()}
	reply := make([]byte, 64+len(payload)+8)
	start := time.Now()
	n, _, err := procIcmp6SendEcho.Call(h, 0, 0, 0, uintptr(unsafe.Pointer(&src)), uintptr(unsafe.Pointer(&dst)),
		uintptr(unsafe.Pointer(&payload[0])), uintptr(len(payload)), 0,
		uintptr(unsafe.Pointer(&reply[0])), uintptr(len(reply)), uintptr(timeout.Milliseconds()))
	elapsed := time.Since(start)
	if n == 0 {
		return 0, classifyStatus(errno(err))
	}
	if code := reply6Code(reply); code != OK {
		return 0, code
	}
	if rtt := binary.LittleEndian.Uint32(reply[32:36]); rtt > 0 {
		return time.Duration(rtt) * time.Millisecond, OK
	}
	return elapsed, OK
}

// reply6Code reads ICMPV6_ECHO_REPLY.Status. IPV6_ADDRESS_EX is packed to 26
// bytes; the outer struct aligns Status to offset 28 (RoundTripTime at 32).
func reply6Code(reply []byte) Code {
	return classifyStatus(binary.LittleEndian.Uint32(reply[28:32]))
}

func errno(err error) uint32 {
	if e, ok := err.(windows.Errno); ok {
		return uint32(e)
	}
	return 11050
}
