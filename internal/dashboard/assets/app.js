function metric(label, value) {
  return `<div class="metric"><span class="metric-label">${escapeHTML(label)}</span><span class="metric-value">${escapeHTML(String(value))}</span></div>`;
}

function pill(value) {
  return `<span class="pill">${escapeHTML(String(value))}</span>`;
}

function pillMuted(value) {
  return `<span class="pill pill-muted">${escapeHTML(String(value))}</span>`;
}

function pillWarn(value) {
  return `<span class="pill pill-warn">${escapeHTML(String(value))}</span>`;
}

function pillHeuristic(value) {
  return `<span class="pill pill-warn">${escapeHTML(String(value))}</span>`;
}

function pillState(value, state) {
  const css = state ? `pill pill-${state}` : "pill";
  return `<span class="${css}">${escapeHTML(String(value))}</span>`;
}

function escapeHTML(value) {
  return String(value)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;");
}

function renderStatus(target, data) {
  const dashboard = data.dashboard || {};
  const storage = data.storage || {};
  target.innerHTML = `
    <div class="metric-grid">
      ${metric("Status", data.status || "unknown")}
      ${metric("Version", dashboard.version || "dev")}
      ${metric("Uptime", `${dashboard.uptime_seconds || 0}s`)}
      ${metric("Trace", dashboard.trace_enabled ? "enabled" : "disabled")}
      ${metric("Probes", storage.probes || 0)}
      ${metric("Benches", storage.benches || 0)}
      ${metric("Traces", storage.traces || 0)}
    </div>
    <p class="mini">Started at ${escapeHTML(dashboard.started_at || "n/a")} | build ${escapeHTML(dashboard.build_time || "unknown")}</p>
  `;
}

function flattenConfig(data) {
  const listeners = data.listeners || {};
  const dashboard = data.dashboard || {};
  const limits = data.limits || {};
  const timeouts = data.timeouts || data.timeouts_ms || {};
  const logging = data.logging || data.observability || {};
  const tls = data.tls || {};
  const listenerTCP = listeners.tcp || listeners.https_tcp || "disabled";
  const listenerH3 = listeners.h3 || listeners.http3_quic || "disabled";
  const listenerExperimental = listeners.experimental_h3 || listeners.experimental || "disabled";
  const dashboardListen = listeners.dashboard || "disabled";
  const allowRemote = dashboard.allow_remote || dashboard.allow_remote_dashboard;
  const traceDir = logging.trace_dir || (logging.trace_dir_configured ? "configured" : "disabled");
  const accessLogEnabled = logging.access_log ? true : logging.access_log_enabled;
  const tlsConfigured = tls.configured || (tls.cert_configured && tls.key_configured);
  const dashboardTransport = dashboard.transport || (allowRemote ? "https" : "http");
  return [
    ["HTTPS/TCP", listenerTCP],
    ["Real HTTP/3 (quic-go)", listenerH3],
    ["Experimental UDP H3 (lab)", listenerExperimental],
    ["Dashboard", dashboardListen],
    ["Dashboard Transport", dashboardTransport],
    ["Dashboard Auth", dashboard.auth_enabled ? "enabled" : "disabled"],
    ["Remote Dashboard", allowRemote ? "enabled" : "disabled"],
    ["Max Body", limits.max_body_bytes || "n/a"],
    ["Rate Limit", limits.rate_limit || "off"],
    ["Read Timeout", timeouts.read || "n/a"],
    ["Write Timeout", timeouts.write || "n/a"],
    ["Idle Timeout", timeouts.idle || "n/a"],
    ["Access Log", accessLogEnabled ? "enabled" : "disabled"],
    ["Trace Dir", traceDir],
    ["TLS", tlsConfigured ? "configured" : "runtime-managed local"],
  ];
}

function renderConfig(target, data) {
  const rows = flattenConfig(data)
    .map(([label, value]) => metric(label, value))
    .join("");
  target.innerHTML = `<div class="metric-grid">${rows}</div>`;
}

function renderOverview(target, probes, benches) {
  const latestProbe = Array.isArray(probes) && probes.length ? probes[0] : null;
  const latestBench = Array.isArray(benches) && benches.length ? benches[0] : null;
  const probeAnalysis = probeAnalysisForItem(latestProbe);
  const probeSummary = probeAnalysis.support_summary || {};
  const benchSummary = latestBench ? latestBench.summary || {} : {};
  target.innerHTML = `
    <div class="metric-grid">
      ${metric("Latest Probe", latestProbe ? (latestProbe.proto || latestProbe.target || "available") : "none")}
      ${metric("Probe Coverage", typeof probeSummary.coverage_ratio === "number" ? `${Math.round(probeSummary.coverage_ratio * 100)}%` : "n/a")}
      ${metric("Bench Best", benchSummary.best_protocol || "n/a")}
      ${metric("Bench Healthy", benchSummary.healthy_protocols || 0)}
      ${metric("Bench Risk", benchSummary.riskiest_protocol || "n/a")}
    </div>
    <p class="mini">
      ${latestProbe ? `Probe requested ${probeSummary.requested_tests || 0}, available ${probeSummary.available || 0}, not-run ${probeSummary.not_run || 0}, unavailable ${probeSummary.unavailable || 0}.` : "No probe coverage summary yet."}
    </p>
    <p class="mini">
      ${latestBench ? `Bench healthy ${benchSummary.healthy_protocols || 0}, degraded ${benchSummary.degraded_protocols || 0}, failed ${benchSummary.failed_protocols || 0}.` : "No benchmark summary yet."}
    </p>
  `;
}

function numericDelta(current, previous) {
  if (!Number.isFinite(current) || !Number.isFinite(previous)) {
    return "n/a";
  }
  const delta = current - previous;
  const sign = delta > 0 ? "+" : "";
  return `${sign}${delta.toFixed(2)}`;
}

function findProtocolP95(item, name) {
  const protocols = benchProtocolsForItem(item);
  for (const [protocol, stats] of protocols) {
    if (String(protocol).toLowerCase() !== String(name).toLowerCase()) {
      continue;
    }
    const latency = stats && stats.latency_ms ? stats.latency_ms : {};
    if (typeof latency.p95 === "number") {
      return latency.p95;
    }
  }
  return NaN;
}

function renderCompare(target, probes, benches) {
  const latestProbe = Array.isArray(probes) && probes.length > 0 ? probes[0] : null;
  const prevProbe = Array.isArray(probes) && probes.length > 1 ? probes[1] : null;
  const latestBench = Array.isArray(benches) && benches.length > 0 ? benches[0] : null;
  const prevBench = Array.isArray(benches) && benches.length > 1 ? benches[1] : null;

  if (!latestProbe && !latestBench) {
    target.innerHTML = `<p class="empty">No data available for trend comparison yet.</p>`;
    return;
  }

  const latestProbeAnalysis = probeAnalysisForItem(latestProbe);
  const prevProbeAnalysis = probeAnalysisForItem(prevProbe);
  const latestProbeSummary = objectField(latestProbeAnalysis, "support_summary");
  const prevProbeSummary = objectField(prevProbeAnalysis, "support_summary");
  const probeCoverageCurrent = typeof latestProbeSummary.coverage_ratio === "number" ? latestProbeSummary.coverage_ratio * 100 : NaN;
  const probeCoveragePrevious = typeof prevProbeSummary.coverage_ratio === "number" ? prevProbeSummary.coverage_ratio * 100 : NaN;

  const latestBenchSummary = latestBench && latestBench.summary ? latestBench.summary : {};
  const prevBenchSummary = prevBench && prevBench.summary ? prevBench.summary : {};
  const bestProtocol = latestBenchSummary.best_protocol || "";
  const p95Current = bestProtocol ? findProtocolP95(latestBench, bestProtocol) : NaN;
  const p95Previous = bestProtocol ? findProtocolP95(prevBench, bestProtocol) : NaN;

  const benchHealthCurrent = Number(latestBenchSummary.healthy_protocols || 0);
  const benchHealthPrevious = Number(prevBenchSummary.healthy_protocols || 0);

  target.innerHTML = `
    <div class="metric-grid">
      ${metric("Probe Coverage", Number.isFinite(probeCoverageCurrent) ? `${probeCoverageCurrent.toFixed(1)}%` : "n/a")}
      ${metric("Coverage Δ", numericDelta(probeCoverageCurrent, probeCoveragePrevious))}
      ${metric("Bench Healthy", benchHealthCurrent)}
      ${metric("Healthy Δ", numericDelta(benchHealthCurrent, benchHealthPrevious))}
      ${metric("Best Protocol", bestProtocol || "n/a")}
      ${metric("Best P95 Δ", numericDelta(p95Current, p95Previous))}
    </div>
    <div class="pill-row">
      ${latestProbe ? pillMuted(`probe latest ${latestProbe.id || latestProbe.target || "n/a"}`) : ""}
      ${prevProbe ? pillMuted(`probe prev ${prevProbe.id || prevProbe.target || "n/a"}`) : ""}
      ${latestBench ? pillMuted(`bench latest ${latestBench.id || latestBench.target || "n/a"}`) : ""}
      ${prevBench ? pillMuted(`bench prev ${prevBench.id || prevBench.target || "n/a"}`) : ""}
    </div>
    <p class="mini">
      ${bestProtocol ? `Best protocol trend is based on ${escapeHTML(bestProtocol)} p95 latency from stats_view.` : "Best protocol trend becomes available once bench summary includes best protocol."}
    </p>
  `;
}

function probeAnalysisForItem(item) {
  if (!item || typeof item !== "object") {
    return {};
  }
  if (item.analysis_view && typeof item.analysis_view === "object") {
    return item.analysis_view;
  }
  if (item.analysis && typeof item.analysis === "object") {
    return item.analysis;
  }
  return {};
}

function benchProtocolsForItem(item) {
  if (!item || typeof item !== "object") {
    return [];
  }
  if (Array.isArray(item.stats_view) && item.stats_view.length) {
    return item.stats_view
      .filter((entry) => entry && typeof entry === "object")
      .map((entry) => [entry.protocol, entry.stats || {}]);
  }
  const stats = item.stats && typeof item.stats === "object" ? item.stats : {};
  return Object.entries(stats);
}

function objectField(source, key) {
  if (!source || typeof source !== "object") {
    return {};
  }
  const value = source[key];
  return value && typeof value === "object" ? value : {};
}

function arrayField(source, key) {
  if (!source || typeof source !== "object") {
    return [];
  }
  const value = source[key];
  return Array.isArray(value) ? value : [];
}

function joinNonEmpty(parts) {
  return parts.filter(Boolean).join("");
}

function fidelityDefinitions(summary) {
  const definitions = summary && typeof summary === "object" ? summary.definitions : null;
  if (definitions && typeof definitions === "object" && Object.keys(definitions).length) {
    return definitions;
  }
  return {
    full: { label: "full", description: "direct current-path diagnostics" },
    observed: { label: "observed", description: "visible protocol/client-layer observation" },
    partial: { label: "partial", description: "heuristic, estimate, or capability-check output" },
    unavailable: { label: "unavailable", description: "requested but not available on the current path" },
  };
}

function fidelityLegend(summary) {
  const definitions = fidelityDefinitions(summary);
  return [
    `full=${definitions.full.description}`,
    `observed=${definitions.observed.description}`,
    `partial=${definitions.partial.description}`,
  ].join("; ");
}

function fidelityState(summary) {
  if (!summary || typeof summary !== "object") {
    return "n/a";
  }
  const partial = Array.isArray(summary.partial) ? summary.partial.length : 0;
  const observed = Array.isArray(summary.observed) ? summary.observed.length : 0;
  if (partial > 0 || observed > 0 || summary.packet_level === false) {
    return "mixed";
  }
  return "full";
}

function buildPlanPills(requested, executed, skipped) {
  return joinNonEmpty([
    requested.length ? pillMuted(`requested ${requested.join(",")}`) : "",
    executed.length ? pill(`executed ${executed.join(",")}`) : "",
    ...skipped.map((entry) => pillWarn(`skipped ${entry.name}`)),
  ]);
}

function buildSupportPills(zeroRTTSupport, migrationSupport) {
  return joinNonEmpty([
    zeroRTTSupport.coverage ? (zeroRTTSupport.coverage === "partial" ? pillHeuristic(`0rtt ${zeroRTTSupport.coverage}`) : pillMuted(`0rtt ${zeroRTTSupport.coverage}`)) : "",
    zeroRTTSupport.state ? pill(zeroRTTSupport.state) : "",
    migrationSupport.coverage ? (migrationSupport.coverage === "partial" ? pillHeuristic(`migration ${migrationSupport.coverage}`) : pillMuted(`migration ${migrationSupport.coverage}`)) : "",
    migrationSupport.state ? pill(migrationSupport.state) : "",
  ]);
}

function buildAdvancedPills(context) {
  const {
    fidelitySummary,
    zeroRTT,
    migration,
    qpack,
    loss,
    congestion,
    version,
    retry,
    ecn,
    spin,
  } = context;
  return joinNonEmpty([
    Array.isArray(fidelitySummary.partial) && fidelitySummary.partial.length ? pillHeuristic(`partial ${fidelitySummary.partial.join(",")}`) : "",
    Array.isArray(fidelitySummary.observed) && fidelitySummary.observed.length ? pillMuted(`observed ${fidelitySummary.observed.join(",")}`) : "",
    zeroRTT.mode ? pillHeuristic(`0rtt ${zeroRTT.resumed ? "resumed" : "checked"}`) : "",
    typeof zeroRTT.time_saved_ms === "number" ? pill(`resume saved ${Number(zeroRTT.time_saved_ms).toFixed(2)}ms`) : "",
    migration.mode ? pillHeuristic(`migration ${migration.supported ? "reachable" : "checked"}`) : "",
    typeof migration.duration_ms === "number" ? pill(`migration ${Number(migration.duration_ms).toFixed(2)}ms`) : "",
    qpack.mode ? pillHeuristic(`qpack ${qpack.mode}`) : "",
    typeof qpack.estimated_ratio === "number" ? pill(`qpack ratio ${Number(qpack.estimated_ratio).toFixed(2)}`) : "",
    loss.mode ? pillHeuristic(`loss ${loss.signal || "checked"}`) : "",
    congestion.mode ? pillHeuristic(`congestion ${congestion.signal || "checked"}`) : "",
    version.mode ? pillHeuristic(`version ${version.alpn || version.observed_proto || "checked"}`) : "",
    retry.mode ? pillHeuristic(`retry ${retry.retry_observed ? "observed" : "not-seen"}`) : "",
    ecn.mode ? pillHeuristic(`ecn ${ecn.ecn_visible ? "visible" : "not-seen"}`) : "",
    spin.mode ? pillHeuristic(`spin ${spin.stability || "checked"}`) : "",
  ]);
}

function buildProbeNotes(context) {
  const {
    skipped,
    fidelitySummary,
    support,
    otherSupport,
    supportSummary,
    zeroRTT,
    migration,
    qpack,
    loss,
    congestion,
    version,
    retry,
    ecn,
    spin,
    zeroRTTSupport,
    migrationSupport,
  } = context;
  return [
    fidelitySummary.notice ? `<p class="mini">Notice: ${escapeHTML(String(fidelitySummary.notice))}</p>` : "",
    fidelitySummary && Object.keys(fidelityDefinitions(fidelitySummary)).length ? `<p class="mini">Fidelity legend: ${escapeHTML(fidelityLegend(fidelitySummary))}</p>` : "",
    otherSupport.length ? `<p class="mini">Advanced support: ${escapeHTML(otherSupport.join(" | "))}</p>` : "",
    supportSummary.requested_tests ? `<p class="mini">Support summary: requested ${supportSummary.requested_tests}, available ${supportSummary.available || 0}, not-run ${supportSummary.not_run || 0}, unavailable ${supportSummary.unavailable || 0}, full ${supportSummary.full || 0}, observed ${supportSummary.observed || 0}, partial ${supportSummary.partial || 0}</p>` : "",
    skipped.length ? `<p class="mini">Skipped: ${escapeHTML(skipped.map((entry) => `${entry.name}: ${entry.reason}`).join(" | "))}</p>` : "",
    zeroRTT.note ? `<p class="mini">0-RTT: ${escapeHTML(String(zeroRTT.note))}</p>` : "",
    migration.note ? `<p class="mini">Migration: ${escapeHTML(String(migration.note))}</p>` : "",
    qpack.note ? `<p class="mini">QPACK: ${escapeHTML(String(qpack.note))}</p>` : "",
    loss.note ? `<p class="mini">Loss: ${escapeHTML(String(loss.note))}</p>` : "",
    congestion.note ? `<p class="mini">Congestion: ${escapeHTML(String(congestion.note))}</p>` : "",
    version.note ? `<p class="mini">Version: ${escapeHTML(String(version.note))}</p>` : "",
    retry.note ? `<p class="mini">Retry: ${escapeHTML(String(retry.note))}</p>` : "",
    ecn.note ? `<p class="mini">ECN: ${escapeHTML(String(ecn.note))}</p>` : "",
    spin.note ? `<p class="mini">Spin Bit: ${escapeHTML(String(spin.note))}</p>` : "",
    zeroRTTSupport.summary ? `<p class="mini">0-RTT support: ${escapeHTML(String(zeroRTTSupport.summary))}</p>` : "",
    migrationSupport.summary ? `<p class="mini">Migration support: ${escapeHTML(String(migrationSupport.summary))}</p>` : "",
  ].join("");
}

function probeMetricValues(latency, streams, response, zeroRTT, migration, supportSummary) {
  const p95 = latency.p95 ? `${Number(latency.p95).toFixed(2)}ms` : "n/a";
  const streamSuccess = `${Math.round((streams.success_rate || 0) * 100)}%`;
  const throughput = `${Math.round(response.throughput_bytes_sec || 0)} B/s`;
  const zeroRTTState = zeroRTT.mode ? (zeroRTT.resumed ? "resumed" : "checked") : "n/a";
  const migrationState = migration.mode ? (migration.supported ? "reachable" : "checked") : "n/a";
  const coverage = typeof supportSummary.coverage_ratio === "number" ? `${Math.round(supportSummary.coverage_ratio * 100)}%` : "n/a";
  const observedCount = Number(supportSummary.observed || 0);
  const partialCount = Number(supportSummary.partial || 0);
  const advancedLabel = partialCount > 0 || observedCount > 0 || zeroRTT.mode || migration.mode ? "mixed" : "full";
  return {
    p95,
    streams: streams.attempted || 0,
    bytes: response.body_bytes || 0,
    zeroRTTState,
    migrationState,
    coverage,
    advancedLabel,
    latencySamples: latency.samples || 0,
    streamSuccess,
    throughput,
  };
}

function benchTopStats(protocols) {
  const top = protocols[0] ? protocols[0][1] : {};
  const reqPerSec = top.req_per_sec ? Number(top.req_per_sec).toFixed(2) : "n/a";
  const errorRate = typeof top.error_rate === "number" ? `${Math.round(top.error_rate * 100)}%` : "n/a";
  const samples = top.sampled_points ? top.sampled_points : 0;
  return { reqPerSec, errorRate, samples };
}

function buildBenchPills(protocols) {
  return protocols.map(([name, stats]) => {
    const p95 = stats && stats.latency_ms ? Number(stats.latency_ms.p95 || 0).toFixed(2) : "0.00";
    return pill(`${name} p95 ${p95}ms`);
  }).join("");
}

function detailRow(label, value) {
  return `<div class="detail-row"><span class="detail-key">${escapeHTML(String(label))}</span><span class="detail-value">${escapeHTML(String(value))}</span></div>`;
}

function selectionStore() {
  if (!window.__tritonSelection) {
    window.__tritonSelection = { probe: "", bench: "", trace: "" };
  }
  return window.__tritonSelection;
}

function selectedId(kind) {
  return selectionStore()[kind] || "";
}

function setSelectedId(kind, id) {
  selectionStore()[kind] = id || "";
}

function displayDuration(value) {
  if (typeof value === "number" && Number.isFinite(value)) {
    return `${(value / 1000000).toFixed(2)}ms`;
  }
  return String(value || "n/a");
}

function encodePath(value) {
  return encodeURIComponent(String(value || ""));
}

function probeRiskSummary(result, analysis) {
  const status = Number(result.status || 0);
  const supportSummary = objectField(analysis, "support_summary");
  const fidelitySummary = objectField(analysis, "fidelity_summary");
  const skipped = arrayField(objectField(analysis, "test_plan"), "skipped");
  if (status >= 500) {
    return { label: "high risk", state: "error", summary: "probe ended with a server-side failure status" };
  }
  if ((supportSummary.unavailable || 0) > 0 || fidelityState(fidelitySummary) === "mixed") {
    return { label: "review fidelity", state: "warn", summary: "advanced results include observed or partial fidelity and unavailable checks" };
  }
  if (skipped.length > 0) {
    return { label: "skipped checks", state: "warn", summary: "some requested checks were skipped on this path" };
  }
  return { label: "stable", state: "ok", summary: "supported checks completed on the current path" };
}

function benchRiskSummary(result) {
  const summary = result && result.summary ? result.summary : {};
  if (Number(summary.failed_protocols || 0) > 0) {
    return { label: "failed protocol", state: "error", summary: "at least one requested protocol failed in this run" };
  }
  if (Number(summary.degraded_protocols || 0) > 0) {
    return { label: "degraded run", state: "warn", summary: "the run completed, but some protocols look degraded" };
  }
  return { label: "healthy", state: "ok", summary: "bench protocols completed without flagged degradation" };
}

function traceRiskSummary(item) {
  const modified = Date.parse(item && item.modified_at ? item.modified_at : "");
  if (!Number.isFinite(modified)) {
    return { label: "unknown recency", state: "warn", summary: "trace recency could not be determined from metadata" };
  }
  const ageHours = (Date.now() - modified) / (1000 * 60 * 60);
  if (ageHours <= 1) {
    return { label: "fresh trace", state: "ok", summary: "trace file was updated within the last hour" };
  }
  if (ageHours <= 24) {
    return { label: "recent trace", state: "muted", summary: "trace file was updated within the last day" };
  }
  return { label: "stale trace", state: "warn", summary: "trace file is older and may not reflect the latest run" };
}

function renderProbeDetail(target, result) {
  const analysis = probeAnalysisForItem(result);
  const latency = objectField(analysis, "latency");
  const streams = objectField(analysis, "streams");
  const response = objectField(analysis, "response");
  const supportSummary = objectField(analysis, "support_summary");
  const fidelitySummary = objectField(analysis, "fidelity_summary");
  const testPlan = objectField(analysis, "test_plan");
  const support = objectField(analysis, "support");
  const risk = probeRiskSummary(result, analysis);
  const supportEntries = Object.entries(support)
    .filter(([, entry]) => entry && typeof entry === "object")
    .slice(0, 8)
    .map(([name, entry]) => detailRow(name, `${entry.coverage || "unknown"} / ${entry.state || "unknown"}`))
    .join("");

  target.innerHTML = `
    <div class="detail-summary">
      <div class="detail-spotlight">
        <div class="detail-head">
          <div class="detail-title">
            <strong>${escapeHTML(result.target || result.id || "probe")}</strong>
            <span class="detail-meta">ID ${escapeHTML(result.id || "n/a")} | proto ${escapeHTML(result.proto || "n/a")} | duration ${escapeHTML(displayDuration(result.duration))}</span>
          </div>
          ${pillState(risk.label, risk.state)}
        </div>
        <div class="metric-grid">
          ${metric("Status", result.status || "n/a")}
          ${metric("Fidelity", fidelityState(fidelitySummary))}
          ${metric("Coverage", typeof supportSummary.coverage_ratio === "number" ? `${Math.round(supportSummary.coverage_ratio * 100)}%` : "n/a")}
          ${metric("P95", latency.p95 ? `${Number(latency.p95).toFixed(2)}ms` : "n/a")}
          ${metric("Streams", streams.attempted || 0)}
          ${metric("Throughput", response.throughput_bytes_sec ? `${Math.round(response.throughput_bytes_sec)} B/s` : "n/a")}
        </div>
        <p class="mini">${escapeHTML(risk.summary)}</p>
      </div>
      <div class="detail-columns">
        <section class="detail-section">
          <h3>Fidelity & Plan</h3>
          <div class="pill-row">
            ${Array.isArray(fidelitySummary.partial) && fidelitySummary.partial.length ? pillHeuristic(`partial ${fidelitySummary.partial.join(",")}`) : ""}
            ${Array.isArray(fidelitySummary.observed) && fidelitySummary.observed.length ? pillMuted(`observed ${fidelitySummary.observed.join(",")}`) : ""}
            ${Array.isArray(testPlan.executed) && testPlan.executed.length ? pill(`executed ${testPlan.executed.join(",")}`) : ""}
          </div>
          <p class="mini">${escapeHTML(fidelityLegend(fidelitySummary))}</p>
          ${fidelitySummary.notice ? `<p class="mini">${escapeHTML(String(fidelitySummary.notice))}</p>` : ""}
          ${Array.isArray(testPlan.skipped) && testPlan.skipped.length ? `<p class="mini">Skipped: ${escapeHTML(testPlan.skipped.map((entry) => `${entry.name}: ${entry.reason}`).join(" | "))}</p>` : ""}
        </section>
        <section class="detail-section">
          <h3>Support Snapshot</h3>
          <div class="detail-list">
            ${detailRow("requested", supportSummary.requested_tests || 0)}
            ${detailRow("available", supportSummary.available || 0)}
            ${detailRow("not-run", supportSummary.not_run || 0)}
            ${detailRow("unavailable", supportSummary.unavailable || 0)}
            ${detailRow("full", supportSummary.full || 0)}
            ${detailRow("observed", supportSummary.observed || 0)}
            ${detailRow("partial", supportSummary.partial || 0)}
          </div>
        </section>
      </div>
      <section class="detail-section">
        <h3>Advanced Checks</h3>
        <div class="detail-list">
          ${supportEntries || `<p class="mini">No advanced support entries captured for this probe.</p>`}
        </div>
      </section>
    </div>
  `;
}

function renderBenchDetail(target, result) {
  const summary = result && result.summary ? result.summary : {};
  const protocols = benchProtocolsForItem(result);
  const risk = benchRiskSummary(result);
  const protocolRows = protocols.slice(0, 8).map(([name, stats]) => {
    const latency = stats && stats.latency_ms ? stats.latency_ms : {};
    return detailRow(name, `req/s ${Number(stats.req_per_sec || 0).toFixed(2)} | p95 ${Number(latency.p95 || 0).toFixed(2)}ms | err ${Math.round(Number(stats.error_rate || 0) * 100)}%`);
  }).join("");

  target.innerHTML = `
    <div class="detail-summary">
      <div class="detail-spotlight">
        <div class="detail-head">
          <div class="detail-title">
            <strong>${escapeHTML(result.target || result.id || "bench")}</strong>
            <span class="detail-meta">ID ${escapeHTML(result.id || "n/a")} | duration ${escapeHTML(displayDuration(result.duration))} | concurrency ${escapeHTML(String(result.concurrency || 0))}</span>
          </div>
          ${pillState(risk.label, risk.state)}
        </div>
        <div class="metric-grid">
          ${metric("Protocols", summary.protocols || protocols.length || 0)}
          ${metric("Healthy", summary.healthy_protocols || 0)}
          ${metric("Degraded", summary.degraded_protocols || 0)}
          ${metric("Failed", summary.failed_protocols || 0)}
          ${metric("Best", summary.best_protocol || "n/a")}
          ${metric("Riskiest", summary.riskiest_protocol || "n/a")}
        </div>
        <p class="mini">${escapeHTML(risk.summary)}</p>
      </div>
      <div class="detail-columns">
        <section class="detail-section">
          <h3>Run Summary</h3>
          <div class="detail-list">
            ${detailRow("target", result.target || "n/a")}
            ${detailRow("duration", displayDuration(result.duration))}
            ${detailRow("concurrency", result.concurrency || 0)}
            ${detailRow("best protocol", summary.best_protocol || "n/a")}
            ${detailRow("riskiest protocol", summary.riskiest_protocol || "n/a")}
          </div>
        </section>
        <section class="detail-section">
          <h3>Protocol Health</h3>
          <div class="detail-list">
            ${protocolRows || `<p class="mini">No protocol stats captured for this bench.</p>`}
          </div>
        </section>
      </div>
    </div>
  `;
}

function renderTraceDetail(target, item) {
  const risk = traceRiskSummary(item);
  const preview = item && item.preview ? item.preview : "(empty trace)";
  target.innerHTML = `
    <div class="detail-summary">
      <div class="detail-spotlight">
        <div class="detail-head">
          <div class="detail-title">
            <strong>${escapeHTML(item.name || "trace")}</strong>
            <span class="detail-meta">Updated ${escapeHTML(item.modified_at || "n/a")} | size ${escapeHTML(String(item.size_bytes || 0))} bytes</span>
          </div>
          ${pillState(risk.label, risk.state)}
        </div>
        <div class="metric-grid">
          ${metric("Size", `${item.size_bytes || 0} bytes`)}
          ${metric("Updated", item.modified_at || "n/a")}
          ${metric("Preview", `${String(preview).length} chars`)}
        </div>
        <p class="mini">${escapeHTML(risk.summary)}</p>
      </div>
      <div class="detail-columns">
        <section class="detail-section">
          <h3>Trace Metadata</h3>
          <div class="detail-list">
            ${detailRow("name", item.name || "n/a")}
            ${detailRow("modified", item.modified_at || "n/a")}
            ${detailRow("size_bytes", item.size_bytes || 0)}
            ${detailRow("meta_url", item.meta_url || "n/a")}
            ${detailRow("download_url", item.download_url || "n/a")}
          </div>
        </section>
        <section class="detail-section">
          <h3>Actions</h3>
          <div class="detail-actions">
            ${item.meta_url ? `<a class="detail-link" href="${escapeHTML(item.meta_url)}" target="_blank" rel="noopener noreferrer">Open metadata</a>` : ""}
            ${item.download_url ? `<a class="detail-link" href="${escapeHTML(item.download_url)}" target="_blank" rel="noopener noreferrer">Open raw trace</a>` : ""}
          </div>
          <p class="mini">Trace detail uses the stored metadata endpoint and keeps raw `.sqlog` access one click away.</p>
        </section>
      </div>
      <section class="detail-section">
        <h3>Preview</h3>
        <pre class="detail-preview">${escapeHTML(preview)}</pre>
      </section>
    </div>
  `;
}

async function loadDetail(kind, id) {
  if (!id) {
    return;
  }
  const target = document.getElementById(kind === "probe" ? "probe-detail" : (kind === "bench" ? "bench-detail" : "trace-detail"));
  if (!target) {
    return;
  }
  const path = kind === "probe" ? `/api/v1/probes/${encodePath(id)}` : (kind === "bench" ? `/api/v1/benches/${encodePath(id)}` : `/api/v1/traces/meta/${encodePath(id)}`);
  target.innerHTML = `<p class="mini">Loading ${escapeHTML(kind)} detail...</p>`;
  try {
    const response = await fetch(path);
    const data = await response.json();
    setSelectedId(kind, id);
    if (kind === "probe") {
      renderProbeDetail(target, data);
    } else if (kind === "bench") {
      renderBenchDetail(target, data);
    } else {
      renderTraceDetail(target, data);
    }
  } catch (error) {
    target.textContent = String(error);
  }
}

function bindDetailButtons(target, kind) {
  const buttons = target.querySelectorAll(`[data-${kind}-id]`);
  buttons.forEach((button) => {
    button.addEventListener("click", () => {
      const id = button.getAttribute(`data-${kind}-id`);
      if (!id) {
        return;
      }
      setSelectedId(kind, id);
      loadDetail(kind, id);
    });
  });
}

function ensureDetailSelection(kind, items) {
  if (!Array.isArray(items) || items.length === 0) {
    return;
  }
  const current = selectedId(kind);
  const match = items.find((item) => item && item.id === current);
  const next = match ? current : items[0].id;
  if (next) {
    loadDetail(kind, next);
  }
}

function renderProbes(target, items) {
  if (!Array.isArray(items) || items.length === 0) {
    const q = document.getElementById("probes-query");
    target.innerHTML = `<p class="empty">${q && q.value.trim() ? "No probes match current filters." : "No probe results yet."}</p>`;
    return;
  }
  target.innerHTML = `<div class="record-list">${items.map((item) => {
    const analysis = probeAnalysisForItem(item);
    const latency = objectField(analysis, "latency");
    const streams = objectField(analysis, "streams");
    const response = objectField(analysis, "response");
    const zeroRTT = objectField(analysis, "0rtt");
    const migration = objectField(analysis, "migration");
    const qpack = objectField(analysis, "qpack");
    const loss = objectField(analysis, "loss");
    const congestion = objectField(analysis, "congestion");
    const version = objectField(analysis, "version");
    const retry = objectField(analysis, "retry");
    const ecn = objectField(analysis, "ecn");
    const spin = objectField(analysis, "spin-bit");
    const support = objectField(analysis, "support");
    const supportSummary = objectField(analysis, "support_summary");
    const fidelitySummary = objectField(analysis, "fidelity_summary");
    const zeroRTTSupport = objectField(support, "0rtt");
    const migrationSupport = objectField(support, "migration");
    const otherSupport = Object.entries(support)
      .filter(([name]) => name !== "0rtt" && name !== "migration")
      .map(([name, entry]) => `${name}:${entry.coverage || "unknown"}`);
    const plan = objectField(analysis, "test_plan");
    const requested = arrayField(plan, "requested");
    const executed = arrayField(plan, "executed");
    const skipped = arrayField(plan, "skipped");
    const planPills = buildPlanPills(requested, executed, skipped);
    const advancedPills = buildAdvancedPills({ fidelitySummary, zeroRTT, migration, qpack, loss, congestion, version, retry, ecn, spin });
    const supportPills = buildSupportPills(zeroRTTSupport, migrationSupport);
    const notes = buildProbeNotes({
      skipped,
      fidelitySummary,
      support,
      otherSupport,
      supportSummary,
      zeroRTT,
      migration,
      qpack,
      loss,
      congestion,
      version,
      retry,
      ecn,
      spin,
      zeroRTTSupport,
      migrationSupport,
    });
    const metrics = probeMetricValues(latency, streams, response, zeroRTT, migration, supportSummary);
    const statusValue = Number(item.status || 0);
    const statusState = statusValue >= 200 && statusValue < 400 ? "ok" : (statusValue >= 400 ? "error" : "muted");
    const selected = item.id && item.id === selectedId("probe");
    return `
      <article class="record">
        <div class="record-head">
          <div class="record-title">
            <h3>${escapeHTML(item.target || item.id || "probe")}</h3>
            <span class="record-meta">ID ${escapeHTML(item.id || "n/a")} | proto ${escapeHTML(item.proto || "n/a")} | duration ${escapeHTML(displayDuration(item.duration))}</span>
          </div>
          <button class="record-action ${selected ? "is-selected" : ""}" data-probe-id="${escapeHTML(item.id || "")}">${selected ? "Selected" : "Inspect"}</button>
        </div>
        <div class="metric-grid">
          ${metric("Status", item.status || "n/a")}
          ${metric("Proto", item.proto || "n/a")}
          ${metric("Total", displayDuration(item.duration))}
          ${metric("P95", metrics.p95)}
          ${metric("Streams", metrics.streams)}
          ${metric("Bytes", metrics.bytes)}
          ${metric("0-RTT", metrics.zeroRTTState)}
          ${metric("Migration", metrics.migrationState)}
          ${metric("Fidelity", fidelityState(fidelitySummary))}
          ${metric("Coverage", metrics.coverage)}
        </div>
        <div class="pill-row">
          ${pillState(`status ${item.status || "n/a"}`, statusState)}
          ${pill(`latency samples ${metrics.latencySamples}`)}
          ${pill(`stream success ${metrics.streamSuccess}`)}
          ${pill(`throughput ${metrics.throughput}`)}
          ${metrics.advancedLabel === "mixed" ? pillHeuristic("advanced metrics include observed/partial results") : pillMuted("advanced metrics are full")}
        </div>
        <div class="pill-row">${supportPills}</div>
        <div class="pill-row">${advancedPills}</div>
        <div class="pill-row">${planPills}</div>
        ${notes}
      </article>
    `;
  }).join("")}</div>`;
  bindDetailButtons(target, "probe");
  ensureDetailSelection("probe", items);
}

function renderBenches(target, items) {
  if (!Array.isArray(items) || items.length === 0) {
    const q = document.getElementById("benches-query");
    target.innerHTML = `<p class="empty">${q && q.value.trim() ? "No benches match current filters." : "No benchmark results yet."}</p>`;
    return;
  }
  target.innerHTML = `<div class="record-list">${items.map((item) => {
    const protocols = benchProtocolsForItem(item);
    const summary = item.summary || {};
    const pills = buildBenchPills(protocols);
    const top = benchTopStats(protocols);
    const healthState = (summary.failed_protocols || 0) > 0 ? "error" : ((summary.degraded_protocols || 0) > 0 ? "warn" : "ok");
    const selected = item.id && item.id === selectedId("bench");
    return `
      <article class="record">
        <div class="record-head">
          <div class="record-title">
            <h3>${escapeHTML(item.target || item.id || "bench")}</h3>
            <span class="record-meta">ID ${escapeHTML(item.id || "n/a")} | duration ${escapeHTML(displayDuration(item.duration))} | concurrency ${escapeHTML(String(item.concurrency || 0))}</span>
          </div>
          <button class="record-action ${selected ? "is-selected" : ""}" data-bench-id="${escapeHTML(item.id || "")}">${selected ? "Selected" : "Inspect"}</button>
        </div>
        <div class="metric-grid">
          ${metric("Duration", displayDuration(item.duration))}
          ${metric("Concurrency", item.concurrency || 0)}
          ${metric("Protocols", protocols.length)}
          ${metric("Top Req/s", top.reqPerSec)}
          ${metric("Top Error", top.errorRate)}
          ${metric("Samples", top.samples)}
          ${metric("Healthy", summary.healthy_protocols || 0)}
          ${metric("Best", summary.best_protocol || "n/a")}
        </div>
        <div class="pill-row">${pillState(`health ${summary.healthy_protocols || 0}/${summary.protocols || protocols.length || 0}`, healthState)}${pills}</div>
        ${summary.protocols ? `<p class="mini">Bench summary: healthy ${summary.healthy_protocols || 0}, degraded ${summary.degraded_protocols || 0}, failed ${summary.failed_protocols || 0}, riskiest ${escapeHTML(String(summary.riskiest_protocol || "n/a"))}</p>` : ""}
      </article>
    `;
  }).join("")}</div>`;
  bindDetailButtons(target, "bench");
  ensureDetailSelection("bench", items);
}

function renderTraces(target, items) {
  if (!Array.isArray(items) || items.length === 0) {
    const q = document.getElementById("traces-query");
    target.innerHTML = `<p class="empty">${q && q.value.trim() ? "No traces match current filters." : "No trace files found."}</p>`;
    return;
  }
  target.innerHTML = `<div class="record-list">${items.map((item) => {
    const selected = item.name && item.name === selectedId("trace");
    return `
    <article class="record">
      <div class="record-head">
        <div class="record-title">
          <h3>${escapeHTML(item.name || "trace")}</h3>
          <span class="record-meta">Updated ${escapeHTML(item.modified_at || "n/a")} | size ${escapeHTML(String(item.size_bytes || 0))} bytes</span>
        </div>
        <button class="record-action ${selected ? "is-selected" : ""}" data-trace-id="${escapeHTML(item.name || "")}">${selected ? "Selected" : "Inspect"}</button>
      </div>
      <div class="pill-row">${traceRecencyPill(item.modified_at)}</div>
      <div class="metric-grid">
        ${metric("Size", `${item.size_bytes || 0} bytes`)}
        ${metric("Updated", item.modified_at || "n/a")}
      </div>
      <p class="mini">${escapeHTML(item.preview || "")}</p>
    </article>
  `;
  }).join("")}</div>`;
  bindDetailButtons(target, "trace");
  ensureDetailSelection("trace", items.map((item) => ({ id: item.name })));
}

function collectionState(prefix) {
  if (!window.__tritonCollections) {
    window.__tritonCollections = {};
  }
  if (!window.__tritonCollections[prefix]) {
    window.__tritonCollections[prefix] = { offset: 0, limit: 0, total: 0, hasMore: false };
  }
  return window.__tritonCollections[prefix];
}

function pagerHTML(prefix) {
  const state = collectionState(prefix);
  const start = state.total === 0 ? 0 : state.offset + 1;
  const end = state.limit > 0 ? Math.min(state.offset + state.limit, state.total) : state.total;
  return `
    <div class="pager">
      <button id="${prefix}-prev" class="pager-btn" ${state.offset <= 0 ? "disabled" : ""}>Prev</button>
      <span class="pager-status">Showing ${start}-${end} of ${state.total}</span>
      <button id="${prefix}-next" class="pager-btn" ${!state.hasMore ? "disabled" : ""}>Next</button>
    </div>
  `;
}

function traceRecencyPill(modifiedAt) {
  const modified = Date.parse(modifiedAt || "");
  if (!Number.isFinite(modified)) {
    return pillMuted("trace recency unknown");
  }
  const ageHours = (Date.now() - modified) / (1000 * 60 * 60);
  if (ageHours <= 1) {
    return pillState("trace fresh", "ok");
  }
  if (ageHours <= 24) {
    return pillState("trace recent", "muted");
  }
  return pillState("trace stale", "warn");
}

async function load(id, path, render) {
  const target = document.getElementById(id);
  try {
    const response = await fetch(path);
    const data = await response.json();
    render(target, data);
  } catch (error) {
    target.textContent = String(error);
  }
}

function readCollectionFilters(prefix) {
  const state = collectionState(prefix);
  const query = document.getElementById(`${prefix}-query`);
  const sort = document.getElementById(`${prefix}-sort`);
  const limit = document.getElementById(`${prefix}-limit`);
  const params = new URLSearchParams();
  if (query && query.value.trim()) {
    params.set("q", query.value.trim());
  }
  if (sort && sort.value.trim()) {
    params.set("sort", sort.value.trim());
  }
  if (limit && limit.value.trim()) {
    params.set("limit", limit.value.trim());
    state.limit = Number(limit.value.trim()) || 0;
  }
  if (state.offset > 0) {
    params.set("offset", String(state.offset));
  }
  return params.toString();
}

function bindCollectionFilters(prefix, reload) {
  const ids = [`${prefix}-query`, `${prefix}-sort`, `${prefix}-limit`];
  for (const id of ids) {
    const node = document.getElementById(id);
    if (!node) {
      continue;
    }
    const handler = () => {
      collectionState(prefix).offset = 0;
      reload();
    };
    node.addEventListener("input", handler);
    node.addEventListener("change", handler);
  }
}

function bindPager(prefix, reload) {
  const prev = document.getElementById(`${prefix}-prev`);
  const next = document.getElementById(`${prefix}-next`);
  if (prev) {
    prev.addEventListener("click", () => {
      const state = collectionState(prefix);
      const step = state.limit || 0;
      state.offset = Math.max(0, state.offset - step);
      reload();
    });
  }
  if (next) {
    next.addEventListener("click", () => {
      const state = collectionState(prefix);
      const step = state.limit || 0;
      if (state.hasMore && step > 0) {
        state.offset += step;
        reload();
      }
    });
  }
}

async function loadCollection(id, prefix, path, render) {
  const target = document.getElementById(id);
  try {
    const response = await fetch(path);
    const data = await response.json();
    const state = collectionState(prefix);
    state.total = Number(response.headers.get("X-Total-Count") || 0);
    state.hasMore = response.headers.get("X-Has-More") === "true";
    render(target, data);
    target.insertAdjacentHTML("beforeend", pagerHTML(prefix));
    bindPager(prefix, () => {
      const query = readCollectionFilters(prefix);
      loadCollection(id, prefix, `/api/v1/${prefix}${query ? `?${query}` : ""}`, render);
    });
  } catch (error) {
    target.textContent = String(error);
  }
}

async function loadOverview() {
  const target = document.getElementById("overview");
  try {
    const [probesResponse, benchesResponse] = await Promise.all([
      fetch("/api/v1/probes?view=summary"),
      fetch("/api/v1/benches?view=summary"),
    ]);
    const probes = await probesResponse.json();
    const benches = await benchesResponse.json();
    renderOverview(target, probes, benches);
  } catch (error) {
    target.textContent = String(error);
  }
}

async function loadCompare() {
  const target = document.getElementById("compare");
  try {
    const [probesResponse, benchesResponse] = await Promise.all([
      fetch("/api/v1/probes?view=summary&sort=newest&limit=20"),
      fetch("/api/v1/benches?view=summary&sort=newest&limit=20"),
    ]);
    const probes = await probesResponse.json();
    const benches = await benchesResponse.json();
    renderCompare(target, probes, benches);
  } catch (error) {
    target.textContent = String(error);
  }
}

load("status", "/api/v1/status", renderStatus);
load("config", "/api/v1/config", renderConfig);
const reloadProbes = () => {
  const query = readCollectionFilters("probes");
  const prefix = query ? `?view=summary&${query}` : "?view=summary";
  loadCollection("probes", "probes", `/api/v1/probes${prefix}`, renderProbes);
};
const reloadBenches = () => {
  const query = readCollectionFilters("benches");
  const prefix = query ? `?view=summary&${query}` : "?view=summary";
  loadCollection("benches", "benches", `/api/v1/benches${prefix}`, renderBenches);
};
const reloadTraces = () => {
  const query = readCollectionFilters("traces");
  loadCollection("traces", "traces", `/api/v1/traces${query ? `?${query}` : ""}`, renderTraces);
};
bindCollectionFilters("probes", reloadProbes);
bindCollectionFilters("benches", reloadBenches);
bindCollectionFilters("traces", reloadTraces);
reloadProbes();
reloadBenches();
reloadTraces();
loadOverview();
loadCompare();
