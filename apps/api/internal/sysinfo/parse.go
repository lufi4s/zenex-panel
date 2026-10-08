// Package sysinfo reports real host metrics read from the operating system.
// Nothing here is hard-coded: every value comes from /proc or the filesystem.
package sysinfo

import (
	"bufio"
	"errors"
	"strconv"
	"strings"
)

var ErrUnsupported = errors.New("host metrics are only available on Linux")

// ParseLoadavg reads the first three fields of /proc/loadavg.
func ParseLoadavg(s string) (l1, l5, l15 float64, err error) {
	f := strings.Fields(s)
	if len(f) < 3 {
		return 0, 0, 0, errors.New("loadavg: too few fields")
	}
	vals := make([]float64, 3)
	for i := 0; i < 3; i++ {
		vals[i], err = strconv.ParseFloat(f[i], 64)
		if err != nil {
			return 0, 0, 0, err
		}
	}
	return vals[0], vals[1], vals[2], nil
}

// ParseMeminfo returns total and available memory in bytes from /proc/meminfo.
func ParseMeminfo(s string) (total, available uint64, err error) {
	var gotTotal, gotAvail bool
	sc := bufio.NewScanner(strings.NewReader(s))
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 2 {
			continue
		}
		switch fields[0] {
		case "MemTotal:":
			total, err = parseKiB(fields[1])
			gotTotal = true
		case "MemAvailable:":
			available, err = parseKiB(fields[1])
			gotAvail = true
		}
		if err != nil {
			return 0, 0, err
		}
	}
	if !gotTotal || !gotAvail {
		return 0, 0, errors.New("meminfo: MemTotal or MemAvailable missing")
	}
	return total, available, nil
}

// ParseUptime returns system uptime in seconds from /proc/uptime.
func ParseUptime(s string) (float64, error) {
	f := strings.Fields(s)
	if len(f) < 1 {
		return 0, errors.New("uptime: empty")
	}
	return strconv.ParseFloat(f[0], 64)
}

func parseKiB(s string) (uint64, error) {
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, err
	}
	return v * 1024, nil
}
