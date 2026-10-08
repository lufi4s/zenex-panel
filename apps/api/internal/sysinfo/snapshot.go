package sysinfo

import "runtime"

// Metrics is a point-in-time host snapshot.
type Metrics struct {
	CPUCount       int     `json:"cpu_count"`
	Load1          float64 `json:"load_1m"`
	Load5          float64 `json:"load_5m"`
	Load15         float64 `json:"load_15m"`
	MemTotalBytes  uint64  `json:"mem_total_bytes"`
	MemUsedBytes   uint64  `json:"mem_used_bytes"`
	DiskTotalBytes uint64  `json:"disk_total_bytes"`
	DiskUsedBytes  uint64  `json:"disk_used_bytes"`
	UptimeSeconds  float64 `json:"uptime_seconds"`
}

// CPUCount returns the logical CPU count visible to the process.
func CPUCount() int { return runtime.NumCPU() }
