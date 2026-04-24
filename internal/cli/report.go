package cli

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tritonprobe/triton/internal/bench"
	"github.com/tritonprobe/triton/internal/config"
	"github.com/tritonprobe/triton/internal/probe"
	"gopkg.in/yaml.v3"
)

type probeReportOptions struct {
	ProfileName string
	ReportName  string
	Result      probe.Result
	Thresholds  config.ProbeThresholds
}

type benchReportOptions struct {
	ProfileName string
	ReportName  string
	Result      bench.Result
	Thresholds  config.BenchThresholds
}

type thresholdReport struct {
	Passed     bool     `json:"passed" yaml:"passed"`
	Violations []string `json:"violations,omitempty" yaml:"violations,omitempty"`
}

type CheckResult struct {
	GeneratedAt time.Time         `json:"generated_at" yaml:"generated_at"`
	Profile     string            `json:"profile,omitempty" yaml:"profile,omitempty"`
	Passed      bool              `json:"passed" yaml:"passed"`
	Failures    []string          `json:"failures,omitempty" yaml:"failures,omitempty"`
	Probe       *CheckProbeResult `json:"probe,omitempty" yaml:"probe,omitempty"`
	Bench       *CheckBenchResult `json:"bench,omitempty" yaml:"bench,omitempty"`
}

type CheckProbeResult struct {
	Profile    string        `json:"profile,omitempty" yaml:"profile,omitempty"`
	Passed     bool          `json:"passed" yaml:"passed"`
	Violations []string      `json:"violations,omitempty" yaml:"violations,omitempty"`
	Error      string        `json:"error,omitempty" yaml:"error,omitempty"`
	Result     *probe.Result `json:"result,omitempty" yaml:"result,omitempty"`
}

type CheckBenchResult struct {
	Profile    string        `json:"profile,omitempty" yaml:"profile,omitempty"`
	Passed     bool          `json:"passed" yaml:"passed"`
	Violations []string      `json:"violations,omitempty" yaml:"violations,omitempty"`
	Error      string        `json:"error,omitempty" yaml:"error,omitempty"`
	Result     *bench.Result `json:"result,omitempty" yaml:"result,omitempty"`
}

type checkSummary struct {
	Kind        string             `json:"kind"`
	GeneratedAt time.Time          `json:"generated_at"`
	Profile     string             `json:"profile,omitempty"`
	Passed      bool               `json:"passed"`
	Failures    []string           `json:"failures,omitempty"`
	Probe       *checkProbeSummary `json:"probe,omitempty"`
	Bench       *checkBenchSummary `json:"bench,omitempty"`
}

type checkProbeSummary struct {
	Profile    string   `json:"profile,omitempty"`
	Passed     bool     `json:"passed"`
	Target     string   `json:"target,omitempty"`
	ResultID   string   `json:"result_id,omitempty"`
	Status     int      `json:"status,omitempty"`
	Protocol   string   `json:"protocol,omitempty"`
	Violations []string `json:"violations,omitempty"`
	Error      string   `json:"error,omitempty"`
}

type checkBenchSummary struct {
	Profile       string   `json:"profile,omitempty"`
	Passed        bool     `json:"passed"`
	Target        string   `json:"target,omitempty"`
	ResultID      string   `json:"result_id,omitempty"`
	Protocols     []string `json:"protocols,omitempty"`
	BestProtocol  string   `json:"best_protocol,omitempty"`
	RiskyProtocol string   `json:"riskiest_protocol,omitempty"`
	Violations    []string `json:"violations,omitempty"`
	Error         string   `json:"error,omitempty"`
}

type junitTestSuites struct {
	XMLName  xml.Name         `xml:"testsuites"`
	Name     string           `xml:"name,attr,omitempty"`
	Tests    int              `xml:"tests,attr"`
	Failures int              `xml:"failures,attr"`
	Time     string           `xml:"time,attr,omitempty"`
	Suites   []junitTestSuite `xml:"testsuite"`
}

type junitTestSuite struct {
	Name      string          `xml:"name,attr"`
	Tests     int             `xml:"tests,attr"`
	Failures  int             `xml:"failures,attr"`
	Time      string          `xml:"time,attr,omitempty"`
	TestCases []junitTestCase `xml:"testcase"`
}

type junitTestCase struct {
	Name      string        `xml:"name,attr"`
	ClassName string        `xml:"classname,attr,omitempty"`
	Time      string        `xml:"time,attr,omitempty"`
	Failure   *junitFailure `xml:"failure,omitempty"`
	SystemOut string        `xml:"system-out,omitempty"`
}

type junitFailure struct {
	Message string `xml:"message,attr"`
	Body    string `xml:",chardata"`
}

type probeReportEnvelope struct {
	Kind        string                 `json:"kind" yaml:"kind"`
	Name        string                 `json:"name,omitempty" yaml:"name,omitempty"`
	Profile     string                 `json:"profile,omitempty" yaml:"profile,omitempty"`
	GeneratedAt time.Time              `json:"generated_at" yaml:"generated_at"`
	Thresholds  thresholdReport        `json:"thresholds" yaml:"thresholds"`
	Result      probe.Result           `json:"result" yaml:"result"`
	Config      config.ProbeThresholds `json:"threshold_config,omitempty" yaml:"threshold_config,omitempty"`
}

type benchReportEnvelope struct {
	Kind        string                 `json:"kind" yaml:"kind"`
	Name        string                 `json:"name,omitempty" yaml:"name,omitempty"`
	Profile     string                 `json:"profile,omitempty" yaml:"profile,omitempty"`
	GeneratedAt time.Time              `json:"generated_at" yaml:"generated_at"`
	Thresholds  thresholdReport        `json:"thresholds" yaml:"thresholds"`
	Result      bench.Result           `json:"result" yaml:"result"`
	Config      config.BenchThresholds `json:"threshold_config,omitempty" yaml:"threshold_config,omitempty"`
}

func evaluateProbeThresholds(result probe.Result, thresholds config.ProbeThresholds) []string {
	var violations []string
	if thresholds.RequireStatusMin != 0 && result.Status < thresholds.RequireStatusMin {
		violations = append(violations, fmt.Sprintf("status %d < min %d", result.Status, thresholds.RequireStatusMin))
	}
	if thresholds.RequireStatusMax != 0 && result.Status > thresholds.RequireStatusMax {
		violations = append(violations, fmt.Sprintf("status %d > max %d", result.Status, thresholds.RequireStatusMax))
	}
	totalMS := result.Duration.Milliseconds()
	if result.Timings != nil && result.Timings["total"] > 0 {
		totalMS = result.Timings["total"]
	}
	if thresholds.MaxTotalMS != 0 && totalMS > thresholds.MaxTotalMS {
		violations = append(violations, fmt.Sprintf("total %dms > max %dms", totalMS, thresholds.MaxTotalMS))
	}
	if latency, ok := anyToLatencyAnalysis(result.Analysis["latency"]); ok && thresholds.MaxLatencyP95MS != 0 && latency.P95 > thresholds.MaxLatencyP95MS {
		violations = append(violations, fmt.Sprintf("latency p95 %.2fms > max %.2fms", latency.P95, thresholds.MaxLatencyP95MS))
	}
	if streams, ok := anyToStreamAnalysis(result.Analysis["streams"]); ok {
		if thresholds.MaxStreamP95MS != 0 && streams.P95Latency > thresholds.MaxStreamP95MS {
			violations = append(violations, fmt.Sprintf("stream p95 %.2fms > max %.2fms", streams.P95Latency, thresholds.MaxStreamP95MS))
		}
		if thresholds.MinStreamSuccessRate != 0 && streams.SuccessRate < thresholds.MinStreamSuccessRate {
			violations = append(violations, fmt.Sprintf("stream success rate %.2f < min %.2f", streams.SuccessRate, thresholds.MinStreamSuccessRate))
		}
	}
	if summary, ok := anyToSupportSummary(result.Analysis["support_summary"]); ok && thresholds.MinCoverageRatio != 0 && summary.CoverageRatio < thresholds.MinCoverageRatio {
		violations = append(violations, fmt.Sprintf("coverage ratio %.2f < min %.2f", summary.CoverageRatio, thresholds.MinCoverageRatio))
	}
	return violations
}

func evaluateBenchThresholds(result bench.Result, thresholds config.BenchThresholds) []string {
	var violations []string
	for _, protocol := range sortedBenchKeys(result.Stats) {
		stats := result.Stats[protocol]
		if thresholds.RequireAllHealthy && benchProtocolHealth(stats) != "healthy" {
			violations = append(violations, fmt.Sprintf("%s health=%s", protocol, benchProtocolHealth(stats)))
		}
		if thresholds.MaxErrorRate != 0 && stats.ErrorRate > thresholds.MaxErrorRate {
			violations = append(violations, fmt.Sprintf("%s error rate %.2f > max %.2f", protocol, stats.ErrorRate, thresholds.MaxErrorRate))
		}
		if thresholds.MinReqPerSec != 0 && stats.RequestsPerS < thresholds.MinReqPerSec {
			violations = append(violations, fmt.Sprintf("%s req/s %.2f < min %.2f", protocol, stats.RequestsPerS, thresholds.MinReqPerSec))
		}
		if thresholds.MaxP95MS != 0 && stats.Latency.P95 > thresholds.MaxP95MS {
			violations = append(violations, fmt.Sprintf("%s p95 %.2fms > max %.2fms", protocol, stats.Latency.P95, thresholds.MaxP95MS))
		}
	}
	return violations
}

func benchProtocolHealth(stat bench.Stats) string {
	switch {
	case stat.Requests == 0 && stat.Errors > 0:
		return "failed"
	case stat.ErrorRate >= 0.10:
		return "degraded"
	default:
		return "healthy"
	}
}

func writeProbeReport(path, format string, opts probeReportOptions) error {
	violations := evaluateProbeThresholds(opts.Result, opts.Thresholds)
	envelope := probeReportEnvelope{
		Kind:        "probe_report",
		Name:        opts.ReportName,
		Profile:     opts.ProfileName,
		GeneratedAt: time.Now().UTC(),
		Thresholds: thresholdReport{
			Passed:     len(violations) == 0,
			Violations: violations,
		},
		Result: opts.Result,
		Config: opts.Thresholds,
	}
	body, err := renderReportBody(format, envelope, renderProbeReportMarkdown(opts, violations))
	if err != nil {
		return err
	}
	return writeReportFile(path, body)
}

func writeBenchReport(path, format string, opts benchReportOptions) error {
	violations := evaluateBenchThresholds(opts.Result, opts.Thresholds)
	envelope := benchReportEnvelope{
		Kind:        "bench_report",
		Name:        opts.ReportName,
		Profile:     opts.ProfileName,
		GeneratedAt: time.Now().UTC(),
		Thresholds: thresholdReport{
			Passed:     len(violations) == 0,
			Violations: violations,
		},
		Result: opts.Result,
		Config: opts.Thresholds,
	}
	body, err := renderReportBody(format, envelope, renderBenchReportMarkdown(opts, violations))
	if err != nil {
		return err
	}
	return writeReportFile(path, body)
}

func writeCheckReport(path, format string, result CheckResult) error {
	body, err := renderReportBody(format, result, renderCheckMarkdown(result))
	if err != nil {
		return err
	}
	return writeReportFile(path, body)
}

func writeCheckSummary(path string, result CheckResult) error {
	summary := checkSummary{
		Kind:        "check_summary",
		GeneratedAt: result.GeneratedAt,
		Profile:     result.Profile,
		Passed:      result.Passed,
		Failures:    append([]string(nil), result.Failures...),
	}
	if result.Probe != nil {
		probeSummary := &checkProbeSummary{
			Profile:    result.Probe.Profile,
			Passed:     result.Probe.Passed,
			Violations: append([]string(nil), result.Probe.Violations...),
			Error:      result.Probe.Error,
		}
		if result.Probe.Result != nil {
			probeSummary.Target = result.Probe.Result.Target
			probeSummary.ResultID = result.Probe.Result.ID
			probeSummary.Status = result.Probe.Result.Status
			probeSummary.Protocol = result.Probe.Result.Proto
		}
		summary.Probe = probeSummary
	}
	if result.Bench != nil {
		benchSummary := &checkBenchSummary{
			Profile:    result.Bench.Profile,
			Passed:     result.Bench.Passed,
			Violations: append([]string(nil), result.Bench.Violations...),
			Error:      result.Bench.Error,
		}
		if result.Bench.Result != nil {
			benchSummary.Target = result.Bench.Result.Target
			benchSummary.ResultID = result.Bench.Result.ID
			benchSummary.Protocols = append([]string(nil), result.Bench.Result.Protocols...)
			benchSummary.BestProtocol = result.Bench.Result.Summary.BestProtocol
			benchSummary.RiskyProtocol = result.Bench.Result.Summary.RiskiestProtocol
		}
		summary.Bench = benchSummary
	}
	body, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return err
	}
	return writeReportFile(path, body)
}

func writeCheckJUnitReport(path string, result CheckResult) error {
	suites := buildCheckJUnitReport(result)
	body, err := xml.MarshalIndent(suites, "", "  ")
	if err != nil {
		return err
	}
	body = append([]byte(xml.Header), body...)
	body = append(body, '\n')
	return writeReportFile(path, body)
}

func renderReportBody(format string, envelope any, markdown string) ([]byte, error) {
	switch format {
	case "", "markdown":
		return []byte(markdown), nil
	case "json":
		return json.MarshalIndent(envelope, "", "  ")
	case "yaml":
		return yaml.Marshal(envelope)
	default:
		return nil, fmt.Errorf("unsupported report format %q", format)
	}
}

func renderProbeReportMarkdown(opts probeReportOptions, violations []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Probe Execution Report\n\n")
	writeReportHeader(&b, opts.ReportName, opts.ProfileName, opts.Result.Target)
	writeThresholdSection(&b, violations)
	b.WriteString("## Result\n\n")
	b.WriteString(renderProbeMarkdown(opts.Result))
	return b.String()
}

func renderBenchReportMarkdown(opts benchReportOptions, violations []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Bench Execution Report\n\n")
	writeReportHeader(&b, opts.ReportName, opts.ProfileName, opts.Result.Target)
	writeThresholdSection(&b, violations)
	b.WriteString("## Result\n\n")
	b.WriteString(renderBenchMarkdown(opts.Result))
	return b.String()
}

func writeReportHeader(b *strings.Builder, name, profile, target string) {
	if name != "" {
		fmt.Fprintf(b, "- Name: `%s`\n", name)
	}
	if profile != "" {
		fmt.Fprintf(b, "- Profile: `%s`\n", profile)
	}
	fmt.Fprintf(b, "- Target: `%s`\n", target)
	fmt.Fprintf(b, "- Generated: `%s`\n\n", time.Now().UTC().Format(time.RFC3339))
}

func writeThresholdSection(b *strings.Builder, violations []string) {
	b.WriteString("## Thresholds\n\n")
	if len(violations) == 0 {
		b.WriteString("- Status: `passed`\n\n")
		return
	}
	b.WriteString("- Status: `failed`\n")
	for _, violation := range violations {
		fmt.Fprintf(b, "- Violation: `%s`\n", violation)
	}
	b.WriteString("\n")
}

func writeReportFile(path string, body []byte) error {
	if path == "" {
		return nil
	}
	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return err
		}
	}
	return os.WriteFile(path, body, 0o600)
}

func buildCheckJUnitReport(result CheckResult) junitTestSuites {
	cases := make([]junitTestCase, 0, 2)
	failures := 0

	if result.Probe != nil {
		testCase := junitTestCase{
			Name:      "probe",
			ClassName: "triton.check",
			Time:      formatJUnitSeconds(probeDuration(result.Probe)),
			SystemOut: renderCheckPhaseOutput("probe", result.Probe.Profile, result.Probe.Violations, result.Probe.Error),
		}
		if !result.Probe.Passed {
			failures++
			testCase.Failure = &junitFailure{
				Message: checkFailureMessage(result.Probe.Violations, result.Probe.Error),
				Body:    strings.Join(prefixViolations("probe", result.Probe.Violations), "\n"),
			}
			if result.Probe.Error != "" {
				testCase.Failure.Body = strings.TrimSpace(strings.Join([]string{testCase.Failure.Body, "error: " + result.Probe.Error}, "\n"))
			}
		}
		cases = append(cases, testCase)
	}

	if result.Bench != nil {
		testCase := junitTestCase{
			Name:      "bench",
			ClassName: "triton.check",
			Time:      formatJUnitSeconds(benchDuration(result.Bench)),
			SystemOut: renderCheckBenchOutput(result.Bench),
		}
		if !result.Bench.Passed {
			failures++
			testCase.Failure = &junitFailure{
				Message: checkFailureMessage(result.Bench.Violations, result.Bench.Error),
				Body:    strings.Join(prefixViolations("bench", result.Bench.Violations), "\n"),
			}
			if result.Bench.Error != "" {
				testCase.Failure.Body = strings.TrimSpace(strings.Join([]string{testCase.Failure.Body, "error: " + result.Bench.Error}, "\n"))
			}
		}
		cases = append(cases, testCase)
	}

	return junitTestSuites{
		Name:     "triton-check",
		Tests:    len(cases),
		Failures: failures,
		Time:     formatJUnitSeconds(totalCheckDuration(result)),
		Suites: []junitTestSuite{
			{
				Name:      "triton.check",
				Tests:     len(cases),
				Failures:  failures,
				Time:      formatJUnitSeconds(totalCheckDuration(result)),
				TestCases: cases,
			},
		},
	}
}

func renderCheckPhaseOutput(name, profile string, violations []string, errText string) string {
	parts := []string{fmt.Sprintf("phase=%s", name)}
	if profile != "" {
		parts = append(parts, fmt.Sprintf("profile=%s", profile))
	}
	if len(violations) > 0 {
		parts = append(parts, "violations="+strings.Join(violations, "; "))
	}
	if errText != "" {
		parts = append(parts, "error="+errText)
	}
	return strings.Join(parts, "\n")
}

func renderCheckBenchOutput(result *CheckBenchResult) string {
	if result == nil {
		return ""
	}
	out := renderCheckPhaseOutput("bench", result.Profile, result.Violations, result.Error)
	if result.Result == nil {
		return out
	}
	parts := []string{out}
	parts = append(parts, fmt.Sprintf("target=%s", result.Result.Target))
	if len(result.Result.Protocols) > 0 {
		parts = append(parts, "protocols="+strings.Join(result.Result.Protocols, ","))
	}
	if result.Result.Summary.BestProtocol != "" {
		parts = append(parts, "best_protocol="+result.Result.Summary.BestProtocol)
	}
	if result.Result.Summary.RiskiestProtocol != "" {
		parts = append(parts, "riskiest_protocol="+result.Result.Summary.RiskiestProtocol)
	}
	return strings.Join(parts, "\n")
}

func checkFailureMessage(violations []string, errText string) string {
	if len(violations) > 0 {
		return violations[0]
	}
	if errText != "" {
		return errText
	}
	return "phase failed"
}

func formatJUnitSeconds(d time.Duration) string {
	return fmt.Sprintf("%.3f", d.Seconds())
}

func totalCheckDuration(result CheckResult) time.Duration {
	return probeDuration(result.Probe) + benchDuration(result.Bench)
}

func probeDuration(result *CheckProbeResult) time.Duration {
	if result == nil || result.Result == nil {
		return 0
	}
	return result.Result.Duration
}

func benchDuration(result *CheckBenchResult) time.Duration {
	if result == nil || result.Result == nil {
		return 0
	}
	return result.Result.Duration
}
