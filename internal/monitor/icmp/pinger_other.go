//go:build !linux && !windows

package icmp

import "errors"

// New is not supported on this platform.
func New() (Pinger, error) {
	return nil, errors.New("ICMP nativ nu este suportat pe această platformă")
}
