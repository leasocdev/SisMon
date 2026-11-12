//go:build linux

package icmp

import (
	"context"
	"net"
	"net/netip"
	"os"
	"sync/atomic"
	"time"

	xicmp "golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

type linuxPinger struct {
	privileged bool
	seq        atomic.Uint32
}

// New probes for an unprivileged ICMP socket, then a raw socket
// (CAP_NET_RAW). Without either, every check reports a permission error.
func New() (Pinger, error) {
	if c, err := xicmp.ListenPacket("udp4", "0.0.0.0"); err == nil {
		c.Close()
		return &linuxPinger{}, nil
	}
	if c, err := xicmp.ListenPacket("ip4:icmp", "0.0.0.0"); err == nil {
		c.Close()
		return &linuxPinger{privileged: true}, nil
	}
	return unavailable{}, nil
}

func (p *linuxPinger) Mode() string {
	if p.privileged {
		return "linux-raw"
	}
	return "linux-unprivileged"
}

func (p *linuxPinger) Ping(ctx context.Context, addr netip.Addr, packets int, timeout time.Duration) Result {
	addr = addr.Unmap()
	v6 := addr.Is6()
	network, laddr := "udp4", "0.0.0.0"
	switch {
	case p.privileged && v6:
		network, laddr = "ip6:ipv6-icmp", "::"
	case p.privileged:
		network = "ip4:icmp"
	case v6:
		network, laddr = "udp6", "::"
	}
	conn, err := xicmp.ListenPacket(network, laddr)
	if err != nil {
		return Result{Sent: packets, Code: classifyErr(err)}
	}
	defer conn.Close()
	var dst net.Addr = &net.UDPAddr{IP: addr.AsSlice()}
	if p.privileged {
		dst = &net.IPAddr{IP: addr.AsSlice()}
	}
	id := os.Getpid() & 0xffff
	return run(ctx, packets, timeout, func(ctx context.Context, _ int, timeout time.Duration) (time.Duration, Code) {
		seq := int(p.seq.Add(1) & 0xffff)
		var typ xicmp.Type = ipv4.ICMPTypeEcho
		proto := 1
		if v6 {
			typ, proto = ipv6.ICMPTypeEchoRequest, 58
		}
		msg := xicmp.Message{Type: typ, Body: &xicmp.Echo{ID: id, Seq: seq, Data: []byte("SisMon")}}
		b, err := msg.Marshal(nil)
		if err != nil {
			return 0, Internal
		}
		deadline := time.Now().Add(timeout)
		if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
			deadline = d
		}
		conn.SetDeadline(deadline)
		start := time.Now()
		if _, err := conn.WriteTo(b, dst); err != nil {
			return 0, classifyErr(err)
		}
		buf := make([]byte, 1500)
		for {
			n, peer, err := conn.ReadFrom(buf)
			if err != nil {
				return 0, classifyErr(err)
			}
			rm, err := xicmp.ParseMessage(proto, buf[:n])
			if err != nil {
				continue
			}
			switch body := rm.Body.(type) {
			case *xicmp.Echo:
				if rm.Type != ipv4.ICMPTypeEchoReply && rm.Type != ipv6.ICMPTypeEchoReply {
					continue
				}
				if body.Seq != seq || (p.privileged && body.ID != id) || !samePeer(peer, addr) {
					continue
				}
				return time.Since(start), OK
			case *xicmp.DstUnreach:
				if matchesOriginal(body.Data, v6, id, seq, p.privileged) {
					return 0, classifyDestUnreach(rm.Code)
				}
			}
		}
	})
}

func samePeer(peer net.Addr, want netip.Addr) bool {
	var ip net.IP
	switch a := peer.(type) {
	case *net.UDPAddr:
		ip = a.IP
	case *net.IPAddr:
		ip = a.IP
	}
	got, ok := netip.AddrFromSlice(ip)
	return ok && got.Unmap() == want
}

// matchesOriginal checks the echo header quoted inside an ICMP error.
func matchesOriginal(data []byte, v6 bool, id, seq int, checkID bool) bool {
	hdr := 40
	if !v6 {
		if len(data) < 1 {
			return false
		}
		hdr = int(data[0]&0x0f) * 4
	}
	if len(data) < hdr+8 {
		return false
	}
	e := data[hdr:]
	gotID, gotSeq := int(e[4])<<8|int(e[5]), int(e[6])<<8|int(e[7])
	return gotSeq == seq && (!checkID || gotID == id)
}
