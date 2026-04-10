package bench

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tritonprobe/triton/internal/config"
)

type Result struct {
	ID          string           `json:"id" yaml:"id"`
	Target      string           `json:"target" yaml:"target"`
	Timestamp   time.Time        `json:"timestamp" yaml:"timestamp"`
	Duration    time.Duration    `json:"duration" yaml:"duration"`
	Protocols   []string         `json:"protocols" yaml:"protocols"`
	Concurrency int              `json:"concurrency" yaml:"concurrency"`
	Stats       map[string]Stats `json:"stats" yaml:"stats"`
}

type Stats struct {
	Requests     int64   `json:"requests" yaml:"requests"`
	Errors       int64   `json:"errors" yaml:"errors"`
	AverageMS    float64 `json:"avg_ms" yaml:"avg_ms"`
	RequestsPerS float64 `json:"req_per_sec" yaml:"req_per_sec"`
	Transferred  int64   `json:"bytes" yaml:"bytes"`
}

func Run(target string, cfg config.BenchConfig) (*Result, error) {
	stats := make(map[string]Stats, len(cfg.DefaultProtocols))
	for _, protocol := range cfg.DefaultProtocols {
		run, err := runProtocol(target, protocol, cfg.DefaultDuration, cfg.DefaultConcurrency)
		if err != nil {
			return nil, err
		}
		stats[protocol] = run
	}
	return &Result{
		ID:          fmt.Sprintf("bn-%s", time.Now().UTC().Format("20060102-150405")),
		Target:      target,
		Timestamp:   time.Now().UTC(),
		Duration:    cfg.DefaultDuration,
		Protocols:   append([]string(nil), cfg.DefaultProtocols...),
		Concurrency: cfg.DefaultConcurrency,
		Stats:       stats,
	}, nil
}

func runProtocol(target, protocol string, duration time.Duration, concurrency int) (Stats, error) {
	var requests, errorsCount, totalMS, bytesRead int64

	transport := &http.Transport{
		ForceAttemptHTTP2: protocol == "h2",
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: true},
	}
	if protocol == "h1" {
		transport.ForceAttemptHTTP2 = false
		transport.TLSNextProto = map[string]func(string, *tls.Conn) http.RoundTripper{}
		transport.TLSClientConfig.NextProtos = []string{"http/1.1"}
	} else {
		transport.TLSClientConfig.NextProtos = []string{"h2", "http/1.1"}
	}
	client := &http.Client{
		Timeout:   duration + 5*time.Second,
		Transport: transport,
	}

	stop := time.Now().Add(duration)
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for time.Now().Before(stop) {
				start := time.Now()
				resp, err := client.Get(target)
				if err != nil {
					atomic.AddInt64(&errorsCount, 1)
					continue
				}
				n, _ := io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				atomic.AddInt64(&bytesRead, n)
				atomic.AddInt64(&requests, 1)
				atomic.AddInt64(&totalMS, time.Since(start).Milliseconds())
			}
		}()
	}
	wg.Wait()

	reqs := atomic.LoadInt64(&requests)
	avg := 0.0
	if reqs > 0 {
		avg = float64(atomic.LoadInt64(&totalMS)) / float64(reqs)
	}
	return Stats{
		Requests:     reqs,
		Errors:       atomic.LoadInt64(&errorsCount),
		AverageMS:    avg,
		RequestsPerS: float64(reqs) / duration.Seconds(),
		Transferred:  atomic.LoadInt64(&bytesRead),
	}, nil
}
