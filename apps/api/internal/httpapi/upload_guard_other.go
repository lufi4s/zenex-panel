//go:build !linux

package httpapi

import "math"

// freeBytes reports no shortage off Linux. The panel only runs on Linux; this keeps
// the package building on other systems (development on Windows).
func freeBytes(string) (uint64, error) {
	return math.MaxUint64, nil
}
