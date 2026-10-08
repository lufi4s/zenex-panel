package sysinfo

import (
	"fmt"
	"os"
	"runtime"
	"syscall"
)

// Snapshot reads load, memory, root-disk and uptime values from the kernel.
func Snapshot() (Metrics, error) {
	m := Metrics{CPUCount: runtime.NumCPU()}

	raw, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return m, fmt.Errorf("loadavg: %w", err)
	}
	if m.Load1, m.Load5, m.Load15, err = ParseLoadavg(string(raw)); err != nil {
		return m, err
	}

	raw, err = os.ReadFile("/proc/meminfo")
	if err != nil {
		return m, fmt.Errorf("meminfo: %w", err)
	}
	total, avail, err := ParseMeminfo(string(raw))
	if err != nil {
		return m, err
	}
	m.MemTotalBytes = total
	m.MemUsedBytes = total - avail

	raw, err = os.ReadFile("/proc/uptime")
	if err != nil {
		return m, fmt.Errorf("uptime: %w", err)
	}
	if m.UptimeSeconds, err = ParseUptime(string(raw)); err != nil {
		return m, err
	}

	var st syscall.Statfs_t
	if err := syscall.Statfs("/", &st); err != nil {
		return m, fmt.Errorf("statfs /: %w", err)
	}
	bsize := uint64(st.Bsize)
	m.DiskTotalBytes = uint64(st.Blocks) * bsize
	m.DiskUsedBytes = (uint64(st.Blocks) - uint64(st.Bfree)) * bsize

	return m, nil
}
