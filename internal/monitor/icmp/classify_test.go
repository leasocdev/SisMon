package icmp

import (
	"fmt"
	"math"
	"os"
	"syscall"
	"testing"
)

func TestClassifyErr(t *testing.T) {
	cases := []struct {
		err  error
		want Code
	}{
		{syscall.ENETUNREACH, NetUnreachable},
		{fmt.Errorf("write: %w", syscall.EHOSTUNREACH), HostUnreachable},
		{syscall.EPERM, Permission},
		{syscall.EACCES, Permission},
		{os.ErrDeadlineExceeded, Timeout},
		{syscall.ECONNREFUSED, HostUnreachable},
		{syscall.EINVAL, Internal},
	}
	for _, c := range cases {
		if got := classifyErr(c.err); got != c.want {
			t.Errorf("classifyErr(%v)=%q want %q", c.err, got, c.want)
		}
	}
}

func TestClassifyDestUnreachable(t *testing.T) {
	for code, want := range map[int]Code{0: NetUnreachable, 1: HostUnreachable, 3: HostUnreachable, 6: NetUnreachable, 13: HostUnreachable} {
		if got := classifyDestUnreach(code); got != want {
			t.Errorf("code %d: %q want %q", code, got, want)
		}
	}
}

func TestClassifyStatusWindows(t *testing.T) {
	cases := map[uint32]Code{
		0: OK, 11010: Timeout, 11003: HostUnreachable, 11002: NetUnreachable, 11005: HostUnreachable,
		11040: HostUnreachable, 11013: HostUnreachable, 1231: NetUnreachable, 1232: HostUnreachable,
		5: Permission, 87: Internal, 11001: Internal, 11006: Internal, 11008: Internal, 99999: Internal,
		// Review fix: route/VPN loss on Windows is an outage, not a monitor error.
		11050: NetUnreachable, 11012: NetUnreachable, 11018: NetUnreachable, 11032: Timeout,
		11009: HostUnreachable, 11014: HostUnreachable, 11015: HostUnreachable, 11016: HostUnreachable,
	}
	for st, want := range cases {
		if got := classifyStatus(st); got != want {
			t.Errorf("status %d: %q want %q", st, got, want)
		}
	}
}

func TestResultHelpers(t *testing.T) {
	r := Result{Sent: 3, Received: 1}
	if !r.OK() || math.Abs(r.LossPct()-66.67) > 0.01 {
		t.Fatalf("%v %v", r.OK(), r.LossPct())
	}
	if (Result{Sent: 2, Code: Permission}).MonitorError() != true || (Result{Sent: 2, Code: Timeout}).MonitorError() {
		t.Fatal("MonitorError")
	}
	if (Result{}).LossPct() != 0 {
		t.Fatal("zero sent")
	}
}
