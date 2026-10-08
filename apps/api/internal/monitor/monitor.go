// Package monitor records host metrics and website uptime so the panel can show
// history. Everything is measured on the server itself; nothing is hard-coded.
package monitor

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/zenexcloud/zenex-panel/apps/api/internal/store"
	"github.com/zenexcloud/zenex-panel/apps/api/internal/sysinfo"
)

const (
	defaultMetricsEvery = 30 * time.Second
	defaultProbeEvery   = 60 * time.Second
	defaultRetention    = 7 * 24 * time.Hour
	probeConcurrency    = 4
	probeTimeout        = 10 * time.Second
)

// Store is the persistence surface the collector needs.
type Store interface {
	Notify(ctx context.Context, userID, level, title, body string) error
	InsertMetricSample(ctx context.Context, m store.MetricSample) error
	ReadySites(ctx context.Context) ([]store.LiveSite, error)
	InsertSiteCheck(ctx context.Context, c store.SiteCheck) error
	PruneMonitoring(ctx context.Context, keep time.Duration) error
}

// Collector samples host metrics and probes every live website.
type Collector struct {
	Store        Store
	Log          *slog.Logger
	Sample       func() (sysinfo.Metrics, error)
	MetricsEvery time.Duration
	ProbeEvery   time.Duration
	Retention    time.Duration
	// TLSAddr is where HTTPS probes connect. Production uses the local
	// Caddy listener; each certificate is still verified against the site name.
	TLSAddr string
	// RootCAs overrides the trusted roots (tests only). Nil means the system roots.
	RootCAs *x509.CertPool

	client *http.Client

	mu     sync.Mutex
	lastOK map[string]bool // last known state per site, for change notices
}

// New returns a collector with production defaults.
func New(s Store, log *slog.Logger) *Collector {
	return &Collector{
		Store:        s,
		Log:          log,
		Sample:       sysinfo.Snapshot,
		MetricsEvery: defaultMetricsEvery,
		ProbeEvery:   defaultProbeEvery,
		Retention:    defaultRetention,
		TLSAddr:      "127.0.0.1:443",
		lastOK:       map[string]bool{},
	}
}

// Run blocks until ctx is cancelled, sampling and probing on its schedule.
func (c *Collector) Run(ctx context.Context) {
	c.client = c.newClient()
	metrics := time.NewTicker(c.MetricsEvery)
	probes := time.NewTicker(c.ProbeEvery)
	prune := time.NewTicker(time.Hour)
	defer metrics.Stop()
	defer probes.Stop()
	defer prune.Stop()

	c.sampleOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-metrics.C:
			c.sampleOnce(ctx)
		case <-probes.C:
			c.probeOnce(ctx)
		case <-prune.C:
			if err := c.Store.PruneMonitoring(ctx, c.Retention); err != nil && ctx.Err() == nil {
				c.Log.Warn("pruning monitoring history failed", "error", err)
			}
		}
	}
}

// sampleOnce records one host reading. Non-Linux hosts report unsupported and
// are skipped quietly.
func (c *Collector) sampleOnce(ctx context.Context) {
	m, err := c.Sample()
	if err != nil {
		if !errors.Is(err, sysinfo.ErrUnsupported) {
			c.Log.Warn("host metrics sample failed", "error", err)
		}
		return
	}
	err = c.Store.InsertMetricSample(ctx, store.MetricSample{
		Time:           time.Now(),
		CPUCount:       m.CPUCount,
		Load1m:         m.Load1,
		MemUsedBytes:   int64(m.MemUsedBytes),
		MemTotalBytes:  int64(m.MemTotalBytes),
		DiskUsedBytes:  int64(m.DiskUsedBytes),
		DiskTotalBytes: int64(m.DiskTotalBytes),
	})
	if err != nil && ctx.Err() == nil {
		c.Log.Warn("saving metric sample failed", "error", err)
	}
}

// probeOnce checks every live website once, a few at a time.
func (c *Collector) probeOnce(ctx context.Context) {
	sites, err := c.Store.ReadySites(ctx)
	if err != nil {
		if ctx.Err() == nil {
			c.Log.Warn("listing sites for uptime checks failed", "error", err)
		}
		return
	}
	sem := make(chan struct{}, probeConcurrency)
	var wg sync.WaitGroup
	for _, site := range sites {
		wg.Add(1)
		sem <- struct{}{}
		go func(s store.LiveSite) {
			defer wg.Done()
			defer func() { <-sem }()
			check := c.probe(ctx, s)
			c.recordTransition(ctx, s, check)
			if err := c.Store.InsertSiteCheck(ctx, check); err != nil && ctx.Err() == nil {
				c.Log.Warn("saving uptime check failed", "error", err)
			}
		}(site)
	}
	wg.Wait()
}

// probe requests the site over HTTPS. The connection goes to this server, but
// the certificate is verified for the site's own name, so an expired or
// missing certificate counts as down.
func (c *Collector) probe(ctx context.Context, site store.LiveSite) store.SiteCheck {
	result := store.SiteCheck{SiteID: site.ID, LatencyMS: -1}
	reqCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, "https://"+site.Domain+"/", nil)
	if err != nil {
		result.Error = "invalid address"
		return result
	}
	req.Header.Set("User-Agent", "Zenex-Monitor/1")

	started := time.Now()
	resp, err := c.client.Do(req)
	result.LatencyMS = int(time.Since(started).Milliseconds())
	if err != nil {
		result.Error = describeProbeError(err)
		return result
	}
	defer resp.Body.Close()
	result.StatusCode = resp.StatusCode
	result.OK = resp.StatusCode >= 200 && resp.StatusCode < 400
	if !result.OK {
		result.Error = "HTTP " + http.StatusText(resp.StatusCode)
	}
	return result
}

func (c *Collector) newClient() *http.Client {
	dialer := &net.Dialer{Timeout: probeTimeout}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, c.TLSAddr)
		},
		TLSClientConfig:       tlsConfig(c.RootCAs),
		TLSHandshakeTimeout:   probeTimeout,
		ResponseHeaderTimeout: probeTimeout,
		DisableKeepAlives:     true,
	}
	return &http.Client{
		Transport: transport,
		Timeout:   probeTimeout,
		// A redirect (for example HTTP to HTTPS) still proves the site answers.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// describeProbeError turns a Go error into a short, customer-readable reason.
func describeProbeError(err error) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "certificate"):
		return "HTTPS certificate problem"
	case strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline"):
		return "timed out"
	case strings.Contains(msg, "connection refused"):
		return "connection refused"
	default:
		return "unreachable"
	}
}

func tlsConfig(roots *x509.CertPool) *tls.Config {
	return &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
}

// recordTransition tells the site owner when a website goes down or comes back.
// Repeated results with the same state are not announced again, and the first
// result after start is treated as the baseline.
func (c *Collector) recordTransition(ctx context.Context, site store.LiveSite, check store.SiteCheck) {
	c.mu.Lock()
	prev, known := c.lastOK[site.ID]
	c.lastOK[site.ID] = check.OK
	c.mu.Unlock()
	if !known || prev == check.OK || ctx.Err() != nil {
		return
	}
	var err error
	if check.OK {
		err = c.Store.Notify(ctx, site.OwnerID, "success", "Website is back online",
			site.Domain+" is answering again.")
	} else {
		err = c.Store.Notify(ctx, site.OwnerID, "error", "Website is down",
			site.Domain+" is not answering ("+check.Error+"). The panel keeps checking and will tell you when it recovers.")
	}
	if err != nil {
		c.Log.Warn("saving website notification failed", "site_id", site.ID, "error", err)
	}
}
