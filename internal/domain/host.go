// Package domain validates and normalizes configuration input.
package domain

import (
	"net/netip"
	"strings"
)

// ValidHost reports whether h is an IP address or a DNS name safe to monitor.
func ValidHost(h string) bool {
	if h == "" || len(h) > 253 || strings.TrimSpace(h) != h {
		return false
	}
	if _, err := netip.ParseAddr(h); err == nil {
		return true
	}
	labels := strings.Split(h, ".")
	// An all-numeric last label is a mistyped IPv4 address, not a DNS name
	// (RFC 3696 §2: top-level domains are never all-numeric).
	if last := labels[len(labels)-1]; last != "" && strings.Trim(last, "0123456789") == "" {
		return false
	}
	for _, label := range labels {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
				return false
			}
		}
	}
	return true
}
