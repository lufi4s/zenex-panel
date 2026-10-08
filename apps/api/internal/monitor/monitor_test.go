package monitor

import (
	"context"
	"crypto/x509"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/store"
	"github.com/zenexcloud/zenex-panel/apps/api/internal/sysinfo"
)

type fakeStore struct {
	mu      sync.Mutex
	samples []store.MetricSample
	checks  []store.SiteCheck
	sites   []store.LiveSite
}

func (f *fakeStore) InsertMetricSample(_ context.Context, m store.MetricSample) error {
	f.samples = append(f.samples, m)
	return nil
}
func (f *fakeStore) ReadySites(context.Context) ([]store.LiveSite, error) { return f.sites, nil }
func (f *fakeStore) InsertSiteCheck(_ context.Context, c store.SiteCheck) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.checks = append(f.checks, c)
	return nil
}
func (f *fakeStore) PruneMonitoring(context.Context, time.Duration) error { return nil }

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// newTLSCollector points probes at a local HTTPS server and trusts its certificate.
func newTLSCollector(t *testing.T, srv *httptest.Server) *Collector {
	t.Helper()
	roots := x509.NewCertPool()
	roots.AddCert(srv.Certificate())
	c := New(&fakeStore{}, quiet())
	c.TLSAddr = srv.Listener.Addr().String()
	c.RootCAs = roots
	c.client = c.newClient()
	return c
}

func TestProbeReportsHealthySite(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := newTLSCollector(t, srv)
	got := c.probe(context.Background(), store.LiveSite{ID: "s1", Domain: "example.com"})
	if !got.OK || got.StatusCode != 200 || got.Error != "" {
		t.Fatalf("healthy site reported %+v", got)
	}
	if got.LatencyMS < 0 {
		t.Fatalf("latency not measured: %d", got.LatencyMS)
	}
}

func TestProbeCountsServerErrorsAsDown(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	got := newTLSCollector(t, srv).probe(context.Background(), store.LiveSite{ID: "s1", Domain: "example.com"})
	if got.OK || got.StatusCode != 500 {
		t.Fatalf("500 response reported as %+v", got)
	}
}

func TestProbeFailsWhenCertificateIsNotTrusted(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer srv.Close()

	c := New(&fakeStore{}, quiet())
	c.TLSAddr = srv.Listener.Addr().String()
	c.client = c.newClient() // no RootCAs: the test certificate is untrusted
	got := c.probe(context.Background(), store.LiveSite{ID: "s1", Domain: "example.com"})
	if got.OK {
		t.Fatal("untrusted certificate counted as healthy")
	}
	if got.Error != "HTTPS certificate problem" {
		t.Fatalf("error = %q", got.Error)
	}
}

func TestSampleStoresHostMetrics(t *testing.T) {
	fs := &fakeStore{}
	c := New(fs, quiet())
	c.Sample = func() (sysinfo.Metrics, error) {
		return sysinfo.Metrics{CPUCount: 2, Load1: 0.5, MemTotalBytes: 100, MemUsedBytes: 40, DiskTotalBytes: 200, DiskUsedBytes: 50}, nil
	}
	c.sampleOnce(context.Background())
	if len(fs.samples) != 1 || fs.samples[0].MemUsedBytes != 40 || fs.samples[0].Load1m != 0.5 {
		t.Fatalf("sample not stored correctly: %+v", fs.samples)
	}
}

func TestSampleSkipsUnsupportedPlatform(t *testing.T) {
	fs := &fakeStore{}
	c := New(fs, quiet())
	c.Sample = func() (sysinfo.Metrics, error) { return sysinfo.Metrics{}, sysinfo.ErrUnsupported }
	c.sampleOnce(context.Background())
	if len(fs.samples) != 0 {
		t.Fatal("sample stored on an unsupported platform")
	}
}

func TestProbeOnceChecksEverySite(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer srv.Close()

	fs := &fakeStore{sites: []store.LiveSite{{ID: "a", Domain: "example.com"}, {ID: "b", Domain: "example.com"}}}
	roots := x509.NewCertPool()
	roots.AddCert(srv.Certificate())
	c := New(fs, quiet())
	c.TLSAddr = srv.Listener.Addr().String()
	c.RootCAs = roots
	c.client = c.newClient()
	c.probeOnce(context.Background())
	if len(fs.checks) != 2 {
		t.Fatalf("checks recorded = %d, want 2", len(fs.checks))
	}
}
