package bench

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tritonprobe/triton/internal/appmux"
	"github.com/tritonprobe/triton/internal/config"
	"github.com/tritonprobe/triton/internal/h3"
	"github.com/tritonprobe/triton/internal/observability"
	"github.com/tritonprobe/triton/internal/quic/transport"
	"github.com/tritonprobe/triton/internal/realh3"
)

type Result struct {
	ID          string           `json:"id" yaml:"id"`
	Target      string           `json:"target" yaml:"target"`
	Timestamp   time.Time        `json:"timestamp" yaml:"timestamp"`
	Duration    time.Duration    `json:"duration" yaml:"duration"`
	Protocols   []string         `json:"protocols" yaml:"protocols"`
	Concurrency int              `json:"concurrency" yaml:"concurrency"`
	TraceFiles  []string         `json:"trace_files,omitempty" yaml:"trace_files,omitempty"`
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
	before, err := observability.ListQLOGFiles(cfg.TraceDir)
	if err != nil {
		return nil, err
	}
	stats := make(map[string]Stats, len(cfg.DefaultProtocols))
	for _, protocol := range cfg.DefaultProtocols {
		run, err := runProtocol(target, protocol, cfg.DefaultDuration, cfg.DefaultConcurrency, cfg.Insecure, cfg.TraceDir)
		if err != nil {
			return nil, err
		}
		stats[protocol] = run
	}
	after, err := observability.ListQLOGFiles(cfg.TraceDir)
	if err != nil {
		return nil, err
	}
	return &Result{
		ID:          fmt.Sprintf("bn-%s", time.Now().UTC().Format("20060102-150405")),
		Target:      target,
		Timestamp:   time.Now().UTC(),
		Duration:    cfg.DefaultDuration,
		Protocols:   append([]string(nil), cfg.DefaultProtocols...),
		Concurrency: cfg.DefaultConcurrency,
		TraceFiles:  observability.DiffQLOGFiles(before, after),
		Stats:       stats,
	}, nil
}

func runProtocol(target, protocol string, duration time.Duration, concurrency int, insecure bool, traceDir string) (Stats, error) {
	if protocol == "h3" {
		if strings.HasPrefix(target, "triton://") {
			return runLoopbackH3Protocol(target, duration, concurrency)
		}
		return runHTTP3Protocol(target, duration, concurrency, insecure, traceDir)
	}

	var requests, errorsCount, totalMS, bytesRead int64

	transport := &http.Transport{
		ForceAttemptHTTP2: protocol == "h2",
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: insecure, MinVersion: tls.VersionTLS12},
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
	defer transport.CloseIdleConnections()

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
	stats := Stats{
		Requests:     reqs,
		Errors:       atomic.LoadInt64(&errorsCount),
		AverageMS:    avg,
		RequestsPerS: float64(reqs) / duration.Seconds(),
		Transferred:  atomic.LoadInt64(&bytesRead),
	}
	if stats.Requests == 0 && stats.Errors > 0 {
		return stats, errors.New("benchmark failed: all requests errored")
	}
	return stats, nil
}

func runHTTP3Protocol(target string, duration time.Duration, concurrency int, insecure bool, traceDir string) (Stats, error) {
	var requests, errorsCount, totalMS, bytesRead int64

	client, transport := realh3.NewClient(duration+5*time.Second, insecure, traceDir)
	defer transport.Close()

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
	stats := Stats{
		Requests:     reqs,
		Errors:       atomic.LoadInt64(&errorsCount),
		AverageMS:    avg,
		RequestsPerS: float64(reqs) / duration.Seconds(),
		Transferred:  atomic.LoadInt64(&bytesRead),
	}
	if stats.Requests == 0 && stats.Errors > 0 {
		return stats, errors.New("benchmark failed: all requests errored")
	}
	return stats, nil
}

func runLoopbackH3Protocol(target string, duration time.Duration, concurrency int) (Stats, error) {
	address, path, loopbackOnly, err := parseTritonTarget(target)
	if err != nil {
		return Stats{}, err
	}

	var requests, errorsCount, totalMS, bytesRead int64
	stop := time.Now().Add(duration)

	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for time.Now().Before(stop) {
				start := time.Now()
				resp, err := runSingleH3Request(address, path, duration, loopbackOnly)
				if err != nil {
					atomic.AddInt64(&errorsCount, 1)
					continue
				}
				atomic.AddInt64(&bytesRead, int64(len(resp.Body)))
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
	stats := Stats{
		Requests:     reqs,
		Errors:       atomic.LoadInt64(&errorsCount),
		AverageMS:    avg,
		RequestsPerS: float64(reqs) / duration.Seconds(),
		Transferred:  atomic.LoadInt64(&bytesRead),
	}
	if stats.Requests == 0 && stats.Errors > 0 {
		return stats, errors.New("benchmark failed: all requests errored")
	}
	return stats, nil
}

func parseTritonTarget(target string) (address string, path string, loopbackOnly bool, err error) {
	parsed, err := url.Parse(target)
	if err != nil {
		return "", "", false, err
	}
	if parsed.Scheme != "triton" {
		return "", "", false, errors.New("h3 benchmark currently supports only triton:// targets")
	}
	loopbackOnly = parsed.Host == "loopback"
	if parsed.Path == "" {
		path = "/ping"
	} else {
		path = parsed.Path
	}
	return parsed.Host, path, loopbackOnly, nil
}

func runSingleH3Request(address, path string, timeout time.Duration, loopbackOnly bool) (*h3.Response, error) {
	if !loopbackOnly {
		return h3.RoundTripAddress(address, http.MethodGet, path, nil, timeout)
	}

	listener, err := transport.ListenQUIC("127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	defer listener.Close()
	listener.SetAutoEcho(false)

	dialer := transport.NewDialer(timeout)
	session, err := dialer.DialSession(listener.Addr().String())
	if err != nil {
		return nil, err
	}
	defer session.Close()

	serverConn, err := listener.Accept()
	if err != nil {
		return nil, err
	}

	service := h3.NewServer(listener, serverConn, appmux.New())
	client := h3.NewClient(session)
	return service.ServeRoundTrip(client, http.MethodGet, path, nil)
}
