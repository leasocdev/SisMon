package auth

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
)

// Proxy lists reverse proxies whose X-Forwarded-* headers are trusted.
type Proxy struct{ Trusted []netip.Prefix }

// ParseProxies parses IPs or CIDRs.
func ParseProxies(list []string) (Proxy, error) {
	var p Proxy
	for _, s := range list {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if !strings.Contains(s, "/") {
			a, err := netip.ParseAddr(s)
			if err != nil {
				return p, fmt.Errorf("proxy de încredere invalid: %q", s)
			}
			p.Trusted = append(p.Trusted, netip.PrefixFrom(a.Unmap(), a.Unmap().BitLen()))
			continue
		}
		pr, err := netip.ParsePrefix(s)
		if err != nil {
			return p, fmt.Errorf("proxy de încredere invalid: %q", s)
		}
		p.Trusted = append(p.Trusted, pr.Masked())
	}
	return p, nil
}

func (p Proxy) trusted(a netip.Addr) bool {
	for _, pr := range p.Trusted {
		if pr.Contains(a) {
			return true
		}
	}
	return false
}

// ClientInfo is what the server knows about the real client.
type ClientInfo struct {
	IP       netip.Addr
	Scheme   string
	Host     string
	ViaProxy bool
}

func remoteIP(r *http.Request) netip.Addr {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	a, _ := netip.ParseAddr(host)
	return a.Unmap()
}

// Client resolves the client address, scheme and host, honoring forwarded
// headers only from trusted proxies.
func (p Proxy) Client(r *http.Request) ClientInfo {
	c := ClientInfo{IP: remoteIP(r), Scheme: "http", Host: r.Host}
	if r.TLS != nil {
		c.Scheme = "https"
	}
	if !p.trusted(c.IP) {
		return c
	}
	c.ViaProxy = true
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		hops := strings.Split(xff, ",")
		for i := len(hops) - 1; i >= 0; i-- {
			a, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
			if err != nil {
				break
			}
			c.IP = a.Unmap()
			if !p.trusted(c.IP) {
				break
			}
		}
	}
	if proto := strings.ToLower(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))); proto == "https" || proto == "http" {
		c.Scheme = proto
	}
	if h := strings.TrimSpace(r.Header.Get("X-Forwarded-Host")); h != "" {
		c.Host = h
	}
	return c
}

// DirectLoopback is true for a loopback client that did not come through a
// proxy and carries no forwarding headers.
func (c ClientInfo) DirectLoopback(r *http.Request) bool {
	if c.ViaProxy || !c.IP.IsLoopback() {
		return false
	}
	for _, h := range []string{"X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Proto", "Forwarded", "X-Real-Ip"} {
		if r.Header.Get(h) != "" {
			return false
		}
	}
	return true
}

// SameOrigin checks Origin (or Referer) against the effective scheme+host.
func SameOrigin(r *http.Request, c ClientInfo) bool {
	want := c.Scheme + "://" + strings.ToLower(c.Host)
	if o := r.Header.Get("Origin"); o != "" {
		return strings.ToLower(o) == want
	}
	if ref := r.Header.Get("Referer"); ref != "" {
		u, err := url.Parse(ref)
		return err == nil && strings.ToLower(u.Scheme+"://"+u.Host) == want
	}
	return false
}
