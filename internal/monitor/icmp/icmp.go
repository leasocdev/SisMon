// Package icmp sends ICMP echo requests natively (no external ping process)
// and classifies failures.
package icmp

import (
	"context"
	"errors"
	"net/netip"
	"os"
	"syscall"
	"time"
)

// Code classifies the outcome of a check.
type Code string

const (
	OK              Code = ""
	Timeout         Code = "timeout"
	HostUnreachable Code = "host_unreachable"
	NetUnreachable  Code = "net_unreachable"
	DNS             Code = "dns"
	Permission      Code = "permission"
	Internal        Code = "internal"
)

// Result is the outcome of one check (several packets).
type Result struct {
	Sent      int      `json:"sent"`
	Received  int      `json:"received"`
	LatencyMs *float64 `json:"latency_ms"`
	Code      Code     `json:"code"`
}

// OK reports whether at least one packet was answered.
func (r Result) OK() bool { return r.Received > 0 }

// MonitorError reports a local failure of the monitor, not of the target.
func (r Result) MonitorError() bool { return r.Code == Permission || r.Code == Internal }

// LossPct is the share of unanswered packets.
func (r Result) LossPct() float64 {
	if r.Sent == 0 {
		return 0
	}
	return 100 * float64(r.Sent-r.Received) / float64(r.Sent)
}

// Pinger sends ICMP echo requests.
type Pinger interface {
	Ping(ctx context.Context, addr netip.Addr, packets int, timeout time.Duration) Result
	Mode() string
}

// PacketGap is the pause between packets of one check.
const PacketGap = 200 * time.Millisecond

// probe is one echo attempt: round-trip time or a failure code.
type probe func(ctx context.Context, seq int, timeout time.Duration) (time.Duration, Code)

// run sends packets sequentially and aggregates the result. A monitor error
// aborts the check immediately.
func run(ctx context.Context, packets int, timeout time.Duration, p probe) Result {
	r := Result{Code: OK}
	var total time.Duration
	last := OK
	for i := 0; i < packets; i++ {
		if i > 0 {
			select {
			case <-ctx.Done():
				return finish(r, total, Internal)
			case <-time.After(PacketGap):
			}
		}
		r.Sent++
		rtt, code := p(ctx, i, timeout)
		if code == OK {
			r.Received++
			total += rtt
			continue
		}
		last = code
		if code == Permission || code == Internal {
			return finish(r, total, code)
		}
	}
	return finish(r, total, last)
}

func finish(r Result, total time.Duration, last Code) Result {
	if r.Received > 0 {
		ms := float64(total.Microseconds()) / 1000 / float64(r.Received)
		r.LatencyMs = &ms
		r.Code = OK
		return r
	}
	r.Code = last
	if last == OK {
		r.Code = Timeout
	}
	return r
}

// classifyErr maps socket errors to codes.
func classifyErr(err error) Code {
	switch {
	case err == nil:
		return OK
	case errors.Is(err, os.ErrDeadlineExceeded), errors.Is(err, context.DeadlineExceeded):
		return Timeout
	case errors.Is(err, syscall.ENETUNREACH), errors.Is(err, syscall.ENETDOWN):
		return NetUnreachable
	case errors.Is(err, syscall.EHOSTUNREACH), errors.Is(err, syscall.EHOSTDOWN), errors.Is(err, syscall.ECONNREFUSED):
		return HostUnreachable
	case errors.Is(err, syscall.EPERM), errors.Is(err, syscall.EACCES):
		return Permission
	}
	return Internal
}

// classifyDestUnreach maps ICMP Destination Unreachable codes.
func classifyDestUnreach(code int) Code {
	switch code {
	case 0, 6, 9, 11: // net unreachable / unknown / prohibited / TOS
		return NetUnreachable
	}
	return HostUnreachable
}

// classifyStatus maps Windows IP_STATUS / Win32 error codes.
func classifyStatus(st uint32) Code {
	switch st {
	case 0:
		return OK
	case 11010: // IP_REQ_TIMED_OUT
		return Timeout
	case 11032: // IP_NEGOTIATING_IPSEC: no answer yet, like a timeout
		return Timeout
	// IP_DEST_NET_UNREACHABLE, IP_BAD_ROUTE, IP_BAD_DESTINATION, ERROR_NETWORK_UNREACHABLE.
	// IP_GENERAL_FAILURE is what Windows reports when the route or VPN tunnel
	// to the target is gone: an outage of the path, not of the monitor.
	case 11002, 11012, 11018, 11050, 1231:
		return NetUnreachable
	// host/protocol/port unreachable, packet too big, TTL expired (transit and
	// reassembly), parameter problem, source quench, IPv6 dest unreachable,
	// ERROR_HOST_UNREACHABLE
	case 11003, 11004, 11005, 11009, 11013, 11014, 11015, 11016, 11040, 1232:
		return HostUnreachable
	case 5: // ERROR_ACCESS_DENIED
		return Permission
	}
	// IP_BUF_TOO_SMALL, IP_NO_RESOURCES, IP_BAD_OPTION, IP_HW_ERROR, IP_BAD_REQ,
	// IP_OPTION_TOO_BIG and unknown codes are local faults of the monitor.
	return Internal
}

// Fake is a scripted pinger for tests and demo mode.
type Fake struct {
	Fn func(addr netip.Addr) Result
}

// Ping returns Fn(addr).
func (f *Fake) Ping(_ context.Context, addr netip.Addr, _ int, _ time.Duration) Result {
	return f.Fn(addr)
}

// Mode returns "fake".
func (f *Fake) Mode() string { return "fake" }

// unavailable answers every check with a permission error.
type unavailable struct{}

func (unavailable) Ping(_ context.Context, _ netip.Addr, packets int, _ time.Duration) Result {
	return Result{Sent: packets, Code: Permission}
}
func (unavailable) Mode() string { return "unavailable" }
