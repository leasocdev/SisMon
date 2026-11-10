//go:build windows

package icmp

import (
	"encoding/binary"
	"testing"
)

// Review fix: ICMPV6_ECHO_REPLY.Status is at offset 28 (IPV6_ADDRESS_EX is
// packed to 26 bytes, then padded to 4-byte alignment).
func TestParseReply6Status(t *testing.T) {
	buf := make([]byte, 64)
	binary.LittleEndian.PutUint32(buf[28:], 11010)
	if code := reply6Code(buf); code != Timeout {
		t.Fatalf("got %q", code)
	}
	binary.LittleEndian.PutUint32(buf[28:], 11040)
	if code := reply6Code(buf); code != HostUnreachable {
		t.Fatalf("got %q", code)
	}
	binary.LittleEndian.PutUint32(buf[28:], 0)
	if code := reply6Code(buf); code != OK {
		t.Fatalf("got %q", code)
	}
}
