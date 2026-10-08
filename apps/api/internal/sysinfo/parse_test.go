package sysinfo

import "testing"

func TestParseLoadavg(t *testing.T) {
	l1, l5, l15, err := ParseLoadavg("0.52 0.41 0.38 1/233 4567\n")
	if err != nil {
		t.Fatal(err)
	}
	if l1 != 0.52 || l5 != 0.41 || l15 != 0.38 {
		t.Fatalf("got %v %v %v", l1, l5, l15)
	}
	if _, _, _, err := ParseLoadavg("0.1 0.2"); err == nil {
		t.Error("short input accepted")
	}
}

func TestParseMeminfo(t *testing.T) {
	in := "MemTotal:       2033480 kB\nMemFree:         100000 kB\nMemAvailable:   1500000 kB\n"
	total, avail, err := ParseMeminfo(in)
	if err != nil {
		t.Fatal(err)
	}
	if total != 2033480*1024 || avail != 1500000*1024 {
		t.Fatalf("got total=%d avail=%d", total, avail)
	}
	if _, _, err := ParseMeminfo("MemTotal: 1 kB\n"); err == nil {
		t.Error("missing MemAvailable accepted")
	}
}

func TestParseUptime(t *testing.T) {
	v, err := ParseUptime("12345.67 99999.00\n")
	if err != nil || v != 12345.67 {
		t.Fatalf("got %v, %v", v, err)
	}
}
