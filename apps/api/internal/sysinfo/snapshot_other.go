//go:build !linux

package sysinfo

// Snapshot is unavailable outside Linux; the API reports 503 in that case.
func Snapshot() (Metrics, error) { return Metrics{CPUCount: CPUCount()}, ErrUnsupported }
