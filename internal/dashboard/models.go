package dashboard

import (
	"net/http"
	"sync"
	"time"

	"github.com/tritonprobe/triton/internal/bench"
	"github.com/tritonprobe/triton/internal/buildinfo"
	"github.com/tritonprobe/triton/internal/observability"
	"github.com/tritonprobe/triton/internal/probe"
	"github.com/tritonprobe/triton/internal/storage"
)

type Server struct {
	http           *http.Server
	store          *storage.FileStore
	trace          string
	config         map[string]any
	version        string
	buildTime      string
	startedAt      time.Time
	certFile       string
	keyFile        string
	useTLS         bool
	cacheMu        sync.RWMutex
	probeCache     map[string]cachedProbeSummary
	benchCache     map[string]cachedBenchSummary
	probeListCache map[string]cachedProbeList
	benchListCache map[string]cachedBenchList
	traceListCache map[string]cachedTraceList
}

type cachedProbeSummary struct {
	modTime string
	size    int64
	summary ProbeSummary
}

type cachedBenchSummary struct {
	modTime string
	size    int64
	summary BenchSummary
}

type cachedProbeList struct {
	signature string
	items     []ProbeSummary
}

type cachedBenchList struct {
	signature string
	items     []BenchSummary
}

type cachedTraceList struct {
	signature string
	items     []TraceMetadata
}

type TraceMetadata struct {
	Name        string `json:"name"`
	SizeBytes   int64  `json:"size_bytes"`
	ModifiedAt  string `json:"modified_at"`
	DownloadURL string `json:"download_url"`
	MetaURL     string `json:"meta_url"`
	Preview     string `json:"preview"`
}

type ProbeSummary struct {
	ID           string            `json:"id"`
	Target       string            `json:"target"`
	Timestamp    string            `json:"timestamp"`
	Status       int               `json:"status"`
	Proto        string            `json:"proto"`
	Duration     string            `json:"duration"`
	ModTime      string            `json:"mod_time"`
	Size         int64             `json:"size"`
	Analysis     map[string]any    `json:"analysis,omitempty"`
	AnalysisView ProbeAnalysisView `json:"analysis_view,omitempty"`
	TraceFiles   []string          `json:"trace_files,omitempty"`
}

type ProbeAnalysisView struct {
	Response        *probe.ResponseAnalysis       `json:"response,omitempty"`
	Latency         *probe.LatencyAnalysis        `json:"latency,omitempty"`
	Streams         *probe.StreamAnalysis         `json:"streams,omitempty"`
	AltSvc          *probe.AltSvcAnalysis         `json:"alt_svc,omitempty"`
	ZeroRTT         *probe.ZeroRTTAnalysis        `json:"0rtt,omitempty"`
	Migration       *probe.MigrationAnalysis      `json:"migration,omitempty"`
	QPACK           *probe.QPACKAnalysis          `json:"qpack,omitempty"`
	Loss            *probe.LossAnalysis           `json:"loss,omitempty"`
	Congestion      *probe.CongestionAnalysis     `json:"congestion,omitempty"`
	Version         *probe.VersionAnalysis        `json:"version,omitempty"`
	Retry           *probe.RetryAnalysis          `json:"retry,omitempty"`
	ECN             *probe.ECNAnalysis            `json:"ecn,omitempty"`
	SpinBit         *probe.SpinBitAnalysis        `json:"spin-bit,omitempty"`
	Support         map[string]probe.SupportEntry `json:"support,omitempty"`
	SupportSummary  *probe.SupportSummary         `json:"support_summary,omitempty"`
	FidelitySummary *probe.FidelitySummary        `json:"fidelity_summary,omitempty"`
	TestPlan        *probe.TestPlan               `json:"test_plan,omitempty"`
}

type BenchSummary struct {
	ID          string                 `json:"id"`
	Target      string                 `json:"target"`
	Timestamp   string                 `json:"timestamp"`
	Duration    string                 `json:"duration"`
	Concurrency int                    `json:"concurrency"`
	Protocols   []string               `json:"protocols"`
	Summary     bench.Summary          `json:"summary"`
	Stats       map[string]bench.Stats `json:"stats"`
	StatsView   []BenchProtocolView    `json:"stats_view,omitempty"`
	ModTime     string                 `json:"mod_time"`
	Size        int64                  `json:"size"`
	TraceFiles  []string               `json:"trace_files,omitempty"`
}

type BenchProtocolView struct {
	Protocol string      `json:"protocol"`
	Stats    bench.Stats `json:"stats"`
}

type StatusResponse struct {
	Status    string          `json:"status"`
	Dashboard DashboardStatus `json:"dashboard"`
	Storage   StorageStatus   `json:"storage"`
}

type DashboardStatus struct {
	StartedAt     string `json:"started_at"`
	UptimeSeconds int64  `json:"uptime_seconds"`
	TraceEnabled  bool   `json:"trace_enabled"`
	Version       string `json:"version"`
	BuildTime     string `json:"build_time"`
}

type StorageStatus struct {
	Probes  int `json:"probes"`
	Benches int `json:"benches"`
	Traces  int `json:"traces"`
}

type APIErrorResponse struct {
	Status string         `json:"status"`
	Error  APIErrorDetail `json:"error"`
}

type APIErrorDetail struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
}

type listQuery struct {
	Limit  int
	Offset int
	Q      string
	Sort   string
	View   string
}

type Options struct {
	Username  string
	Password  string
	Logger    *observability.ManagedLogger
	TraceDir  string
	Config    map[string]any
	Version   string
	BuildTime string
	CertFile  string
	KeyFile   string
	UseTLS    bool
}

func (o *Options) withDefaults() {
	if o.Version == "" {
		o.Version = buildinfo.Version
	}
	if o.BuildTime == "" {
		o.BuildTime = buildinfo.BuildTime
	}
}
