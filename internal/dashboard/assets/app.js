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

function escapeBreakableHTML(value) {
  const breakAfter = "/:?&=._-#";
  let out = "";
  for (const char of String(value)) {
    out += escapeHTML(char);
    if (breakAfter.includes(char)) {
      out += "<wbr>";
    }
  }
  return out;
}

function renderStatus(target, data) {
  const dashboard = data.dashboard || {};
  const storage = data.storage || {};
  target.innerHTML = `
    ${metric("Service", data.status || "unknown")}
    ${metric("Uptime", `${dashboard.uptime_seconds || 0}s`)}
    ${metric("Probes", storage.probes || 0)}
    ${metric("Benches", storage.benches || 0)}
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
      ${metric("Latest Status", latestProbe ? (latestProbe.status || "n/a") : "none")}
      ${metric("Latest Proto", latestProbe ? (latestProbe.proto || "n/a") : "none")}
      ${metric("Coverage", typeof probeSummary.coverage_ratio === "number" ? `${Math.round(probeSummary.coverage_ratio * 100)}%` : "n/a")}
      ${metric("Best Bench", benchSummary.best_protocol || "n/a")}
    </div>
    <p class="mini">
      ${latestProbe ? `Last probe: ${escapeHTML(latestProbe.target || latestProbe.id || "n/a")}.` : "No probe result yet."}
      ${latestBench ? ` Last bench: healthy ${benchSummary.healthy_protocols || 0}, degraded ${benchSummary.degraded_protocols || 0}, failed ${benchSummary.failed_protocols || 0}.` : " No benchmark result yet."}
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
      ${metric("Coverage Change", numericDelta(probeCoverageCurrent, probeCoveragePrevious))}
      ${metric("Bench Healthy", benchHealthCurrent)}
      ${metric("Healthy Change", numericDelta(benchHealthCurrent, benchHealthPrevious))}
      ${metric("Best Protocol", bestProtocol || "n/a")}
      ${metric("P95 Change", numericDelta(p95Current, p95Previous))}
    </div>
    <p class="mini">
      ${bestProtocol ? `Trend uses the latest ${escapeHTML(bestProtocol)} p95 latency.` : "Trend appears after at least one benchmark summary."}
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
  return `<div class="detail-row"><span class="detail-key">${escapeHTML(String(label))}</span><span class="detail-value">${escapeBreakableHTML(String(value))}</span></div>`;
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
            <strong>${escapeBreakableHTML(result.target || result.id || "probe")}</strong>
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
            <strong>${escapeBreakableHTML(result.target || result.id || "bench")}</strong>
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
            <strong>${escapeBreakableHTML(item.name || "trace")}</strong>
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
    const supportSummary = objectField(analysis, "support_summary");
    const fidelitySummary = objectField(analysis, "fidelity_summary");
    const statusValue = Number(item.status || 0);
    const statusState = statusValue >= 200 && statusValue < 400 ? "ok" : (statusValue >= 400 ? "error" : "muted");
    const coverage = typeof supportSummary.coverage_ratio === "number" ? `${Math.round(supportSummary.coverage_ratio * 100)}%` : "n/a";
    const p95 = latency.p95 ? `${Number(latency.p95).toFixed(2)}ms` : "n/a";
    const selected = item.id && item.id === selectedId("probe");
    return `
      <article class="record">
        <div class="record-head">
          <div class="record-title">
            <h3>${escapeBreakableHTML(item.target || item.id || "probe")}</h3>
            <span class="record-meta">${escapeHTML(item.proto || "n/a")} | ${escapeHTML(displayDuration(item.duration))} | ${escapeHTML(item.timestamp || "")}</span>
          </div>
          <button class="record-action ${selected ? "is-selected" : ""}" data-probe-id="${escapeHTML(item.id || "")}">${selected ? "Selected" : "Details"}</button>
        </div>
        <div class="metric-grid">
          ${metric("Status", item.status || "n/a")}
          ${metric("Proto", item.proto || "n/a")}
          ${metric("P95", p95)}
          ${metric("Coverage", coverage)}
        </div>
        <div class="pill-row">
          ${pillState(`status ${item.status || "n/a"}`, statusState)}
          ${pillMuted(`fidelity ${fidelityState(fidelitySummary)}`)}
        </div>
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
    const top = benchTopStats(protocols);
    const healthState = (summary.failed_protocols || 0) > 0 ? "error" : ((summary.degraded_protocols || 0) > 0 ? "warn" : "ok");
    const selected = item.id && item.id === selectedId("bench");
    return `
      <article class="record">
        <div class="record-head">
          <div class="record-title">
            <h3>${escapeBreakableHTML(item.target || item.id || "bench")}</h3>
            <span class="record-meta">${escapeHTML(displayDuration(item.duration))} | concurrency ${escapeHTML(String(item.concurrency || 0))} | ${escapeHTML(item.timestamp || "")}</span>
          </div>
          <button class="record-action ${selected ? "is-selected" : ""}" data-bench-id="${escapeHTML(item.id || "")}">${selected ? "Selected" : "Details"}</button>
        </div>
        <div class="metric-grid">
          ${metric("Best", summary.best_protocol || "n/a")}
          ${metric("Req/s", top.reqPerSec)}
          ${metric("Errors", top.errorRate)}
          ${metric("Healthy", summary.healthy_protocols || 0)}
        </div>
        <div class="pill-row">
          ${pillState(`health ${summary.healthy_protocols || 0}/${summary.protocols || protocols.length || 0}`, healthState)}
          ${pillMuted(`protocols ${protocols.map(([name]) => name).join(",") || "n/a"}`)}
        </div>
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
          <h3>${escapeBreakableHTML(item.name || "trace")}</h3>
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

function setBenchActionStatus(message, state) {
  setActionStatus("bench-action-status", message, state);
}

function setProbeActionStatus(message, state) {
  setActionStatus("probe-action-status", message, state);
}

function setClearActionStatus(message, state) {
  setActionStatus("clear-action-status", message, state);
}

function setLastResult(title, message, state) {
  const target = document.getElementById("last-result");
  if (!target) {
    return;
  }
  target.classList.remove("is-ok", "is-error");
  if (state) {
    target.classList.add(`is-${state}`);
  }
  target.innerHTML = `<strong>${escapeHTML(title)}</strong>${escapeBreakableHTML(message)}`;
}

function setActionStatus(id, message, state) {
  const target = document.getElementById(id);
  if (!target) {
    return;
  }
  target.classList.remove("is-ok", "is-error");
  if (state) {
    target.classList.add(`is-${state}`);
  }
  target.textContent = message;
}

function dashboardTargetInput() {
  return document.getElementById("dashboard-target") || document.getElementById("probe-target") || document.getElementById("bench-target");
}

function normalizeTargetValue(value) {
  const trimmed = String(value || "").trim();
  if (!trimmed) {
    return "";
  }
  if (/^[a-z][a-z0-9+.-]*:\/\//i.test(trimmed)) {
    return trimmed;
  }
  return `https://${trimmed}`;
}

function dashboardTargetValue() {
  const target = dashboardTargetInput();
  if (!target) {
    return "";
  }
  const normalized = normalizeTargetValue(target.value);
  if (normalized && target.value !== normalized) {
    target.value = normalized;
  }
  updateTargetHelpers(normalized);
  return normalized;
}

function selectedProbeTests() {
  const suite = document.getElementById("probe-suite");
  const value = suite ? suite.value : "default";
  switch (value) {
    case "full":
      return ["handshake", "tls", "latency", "throughput", "streams", "alt-svc", "0rtt", "migration", "ecn", "retry", "version", "qpack", "congestion", "loss", "spin-bit"];
    case "latency":
      return ["handshake", "tls", "latency"];
    default:
      return ["handshake", "tls", "latency", "throughput", "streams", "alt-svc"];
  }
}

function selectedBenchProtocols() {
  return Array.from(document.querySelectorAll('input[name="bench-protocol"]:checked'))
    .map((input) => input.value)
    .filter(Boolean);
}

function clearScopeLabel(scope) {
  switch (scope) {
    case "probes":
      return "probe results";
    case "benches":
      return "benchmark results";
    case "traces":
      return "trace files";
    default:
      return "all dashboard results";
  }
}

function clearMessage(data) {
  const removed = data && data.removed ? data.removed : {};
  const parts = ["probes", "benches", "traces"]
    .filter((key) => Object.prototype.hasOwnProperty.call(removed, key))
    .map((key) => `${removed[key]} ${key}`);
  return parts.length ? `Cleared ${parts.join(", ")}.` : "Nothing to clear.";
}

function updateTargetHelpers(value) {
  const targetValue = normalizeTargetValue(value || (dashboardTargetInput() ? dashboardTargetInput().value : ""));
  const openTarget = document.getElementById("open-target");
  if (openTarget) {
    openTarget.href = targetValue || "https://example.com";
    openTarget.setAttribute("aria-disabled", targetValue ? "false" : "true");
  }
}

function applyTargetPreset(button) {
  const target = dashboardTargetInput();
  if (!target) {
    return;
  }
  const value = button.dataset.targetPreset || "";
  target.value = value;
  updateTargetHelpers(value);
  if (button.dataset.targetInsecure === "true") {
    const probeInsecure = document.getElementById("probe-insecure");
    const benchInsecure = document.getElementById("bench-insecure");
    if (probeInsecure) {
      probeInsecure.checked = true;
    }
    if (benchInsecure) {
      benchInsecure.checked = true;
    }
  }
  target.focus();
}

function probeResultMessage(result) {
  const analysis = probeAnalysisForItem(result);
  const supportSummary = objectField(analysis, "support_summary");
  const timings = result && result.timings_ms ? result.timings_ms : {};
  const tls = result && result.tls ? result.tls : {};
  const total = typeof timings.total === "number" ? `${Number(timings.total).toFixed(2)}ms` : displayDuration(result.duration);
  const coverage = typeof supportSummary.coverage_ratio === "number" ? `${Math.round(supportSummary.coverage_ratio * 100)}% coverage` : "coverage n/a";
  const tlsLabel = tls.version ? `TLS ${tls.version}` : "TLS n/a";
  return `Probe complete: HTTP ${result.status || "n/a"} over ${result.proto || "n/a"} in ${total}.
${tlsLabel}, ${coverage}.`;
}

function benchResultMessage(result) {
  const summary = result && result.summary ? result.summary : {};
  const protocols = benchProtocolsForItem(result);
  const bestName = summary.best_protocol || (protocols[0] ? protocols[0][0] : "n/a");
  const best = protocols.find(([name]) => String(name).toLowerCase() === String(bestName).toLowerCase());
  const stats = best ? best[1] || {} : {};
  const latency = stats.latency_ms || {};
  const reqPerSec = typeof stats.req_per_sec === "number" ? Number(stats.req_per_sec).toFixed(2) : "n/a";
  const p95 = typeof latency.p95 === "number" ? `${Number(latency.p95).toFixed(2)}ms` : "n/a";
  const errors = typeof stats.error_rate === "number" ? `${Math.round(stats.error_rate * 100)}% errors` : "errors n/a";
  return `Bench complete: best ${bestName}, ${reqPerSec} req/s, p95 ${p95}.
Healthy ${summary.healthy_protocols || 0}/${summary.protocols || protocols.length || 0}, ${errors}.`;
}

async function runProbeFromDashboard(event) {
  event.preventDefault();
  const form = event.currentTarget;
  const submit = document.getElementById("probe-submit");
  const timeout = document.getElementById("probe-timeout");
  const streams = document.getElementById("probe-streams");
  const insecure = document.getElementById("probe-insecure");
  const target = dashboardTargetInput();
  const targetValue = dashboardTargetValue();

  if (!targetValue) {
    setProbeActionStatus("Enter a target URL first.", "error");
    if (target) {
      target.focus();
    }
    return;
  }

  const payload = {
    target: targetValue,
    tests: selectedProbeTests(),
    timeout: timeout ? timeout.value : "10s",
    streams: streams ? Number(streams.value || 5) : 5,
    insecure_tls: Boolean(insecure && insecure.checked),
  };

  if (submit) {
    submit.disabled = true;
    submit.textContent = "Running...";
  }
  setProbeActionStatus(`Probing ${payload.target}`, "");

  try {
    const response = await fetch("/api/v1/actions/probe", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    });
    const data = await response.json();
    if (!response.ok) {
      const message = data && data.error && data.error.message ? data.error.message : `probe failed with HTTP ${response.status}`;
      throw new Error(message);
    }
    const result = data.result || {};
    setProbeActionStatus(probeResultMessage(result), "ok");
    setLastResult("Probe", probeResultMessage(result), "ok");
    if (result.id) {
      setSelectedId("probe", result.id);
      const detail = document.getElementById("probe-detail");
      if (detail) {
        renderProbeDetail(detail, result);
      }
    }
    reloadProbes();
    loadOverview();
    loadCompare();
    load("status", "/api/v1/status", renderStatus);
    form.reset();
    if (target) {
      target.value = payload.target;
    }
  } catch (error) {
    setProbeActionStatus(String(error.message || error), "error");
    setLastResult("Probe failed", String(error.message || error), "error");
  } finally {
    if (submit) {
      submit.disabled = false;
      submit.textContent = "Run Probe";
    }
  }
}

async function runBenchFromDashboard(event) {
  event.preventDefault();
  const form = event.currentTarget;
  const submit = document.getElementById("bench-submit");
  const duration = document.getElementById("bench-duration");
  const concurrency = document.getElementById("bench-concurrency");
  const insecure = document.getElementById("bench-insecure");
  const target = dashboardTargetInput();
  const targetValue = dashboardTargetValue();
  const protocols = selectedBenchProtocols();

  if (!targetValue) {
    setBenchActionStatus("Enter a target URL first.", "error");
    if (target) {
      target.focus();
    }
    return;
  }
  if (protocols.length === 0) {
    setBenchActionStatus("Select at least one protocol.", "error");
    return;
  }

  const payload = {
    target: targetValue,
    protocols,
    duration: duration ? duration.value : "3s",
    concurrency: concurrency ? Number(concurrency.value || 4) : 4,
    insecure_tls: Boolean(insecure && insecure.checked),
  };

  if (submit) {
    submit.disabled = true;
    submit.textContent = "Running...";
  }
  setBenchActionStatus(`Running ${payload.protocols.join(",")} against ${payload.target}`, "");

  try {
    const response = await fetch("/api/v1/actions/bench", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    });
    const data = await response.json();
    if (!response.ok) {
      const message = data && data.error && data.error.message ? data.error.message : `bench failed with HTTP ${response.status}`;
      throw new Error(message);
    }
    const result = data.result || {};
    setBenchActionStatus(benchResultMessage(result), "ok");
    setLastResult("Benchmark", benchResultMessage(result), "ok");
    if (result.id) {
      setSelectedId("bench", result.id);
      const detail = document.getElementById("bench-detail");
      if (detail) {
        renderBenchDetail(detail, result);
      }
    }
    reloadBenches();
    loadOverview();
    loadCompare();
    load("status", "/api/v1/status", renderStatus);
    form.reset();
    if (target) {
      target.value = payload.target;
    }
  } catch (error) {
    setBenchActionStatus(String(error.message || error), "error");
    setLastResult("Benchmark failed", String(error.message || error), "error");
  } finally {
    if (submit) {
      submit.disabled = false;
      submit.textContent = "Run Bench";
    }
  }
}

async function runProbeAndBench() {
  const button = document.getElementById("run-both-submit");
  const probeForm = document.getElementById("probe-runner");
  const benchForm = document.getElementById("bench-runner");
  if (!probeForm || !benchForm) {
    return;
  }
  if (button) {
    button.disabled = true;
    button.textContent = "Running...";
  }
  setLastResult("Running", "Probe first, benchmark next.", "");
  try {
    await runProbeFromDashboard({ preventDefault() {}, currentTarget: probeForm });
    await runBenchFromDashboard({ preventDefault() {}, currentTarget: benchForm });
  } finally {
    if (button) {
      button.disabled = false;
      button.textContent = "Run Probe + Bench";
    }
  }
}

async function clearResults(scope) {
  const label = clearScopeLabel(scope);
  if (!window.confirm(`Clear ${label}? This removes stored dashboard history for this workspace.`)) {
    return;
  }
  const buttons = Array.from(document.querySelectorAll("[data-clear-scope]"));
  buttons.forEach((button) => {
    button.disabled = true;
  });
  setClearActionStatus(`Clearing ${label}...`, "");
  try {
    const response = await fetch("/api/v1/actions/clear", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ scope }),
    });
    const data = await response.json();
    if (!response.ok) {
      const message = data && data.error && data.error.message ? data.error.message : `clear failed with HTTP ${response.status}`;
      throw new Error(message);
    }
    setClearActionStatus(clearMessage(data), "ok");
    setLastResult("Cleared", clearMessage(data), "ok");
    if (scope === "all" || scope === "probes") {
      setSelectedId("probe", "");
      const detail = document.getElementById("probe-detail");
      if (detail) {
        detail.textContent = "Select a probe row to inspect it.";
      }
      reloadProbes();
    }
    if (scope === "all" || scope === "benches") {
      setSelectedId("bench", "");
      const detail = document.getElementById("bench-detail");
      if (detail) {
        detail.textContent = "Select a bench row to inspect it.";
      }
      reloadBenches();
    }
    if (scope === "all" || scope === "traces") {
      setSelectedId("trace", "");
      const detail = document.getElementById("trace-detail");
      if (detail) {
        detail.textContent = "Select a trace row to inspect it.";
      }
      reloadTraces();
    }
    loadOverview();
    loadCompare();
    load("status", "/api/v1/status", renderStatus);
  } catch (error) {
    setClearActionStatus(String(error.message || error), "error");
    setLastResult("Clear failed", String(error.message || error), "error");
  } finally {
    buttons.forEach((button) => {
      button.disabled = false;
    });
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
const benchRunner = document.getElementById("bench-runner");
if (benchRunner) {
  benchRunner.addEventListener("submit", runBenchFromDashboard);
}
const probeRunner = document.getElementById("probe-runner");
if (probeRunner) {
  probeRunner.addEventListener("submit", runProbeFromDashboard);
}
const runBothSubmit = document.getElementById("run-both-submit");
if (runBothSubmit) {
  runBothSubmit.addEventListener("click", runProbeAndBench);
}
const dashboardTarget = dashboardTargetInput();
if (dashboardTarget) {
  dashboardTarget.addEventListener("input", () => updateTargetHelpers(dashboardTarget.value));
  dashboardTarget.addEventListener("keydown", (event) => {
    if (event.key === "Enter") {
      event.preventDefault();
      runProbeAndBench();
    }
  });
  updateTargetHelpers(dashboardTarget.value);
}
document.querySelectorAll("[data-target-preset]").forEach((button) => {
  button.addEventListener("click", () => applyTargetPreset(button));
});
document.querySelectorAll("[data-clear-scope]").forEach((button) => {
  button.addEventListener("click", () => clearResults(button.dataset.clearScope || "all"));
});
reloadProbes();
reloadBenches();
reloadTraces();
loadOverview();
loadCompare();
