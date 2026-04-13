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
      ${metric("Uptime", `${dashboard.uptime_seconds || 0}s`)}
      ${metric("Trace", dashboard.trace_enabled ? "enabled" : "disabled")}
      ${metric("Probes", storage.probes || 0)}
      ${metric("Benches", storage.benches || 0)}
      ${metric("Traces", storage.traces || 0)}
    </div>
    <p class="mini">Started at ${escapeHTML(dashboard.started_at || "n/a")}</p>
  `;
}

function flattenConfig(data) {
  const listeners = data.listeners || {};
  const dashboard = data.dashboard || {};
  const limits = data.limits || {};
  const timeouts = data.timeouts || {};
  const logging = data.logging || {};
  const tls = data.tls || {};
  return [
    ["TCP", listeners.tcp || "disabled"],
    ["HTTP/3", listeners.h3 || "disabled"],
    ["Experimental", listeners.experimental_h3 || "disabled"],
    ["Dashboard", listeners.dashboard || "disabled"],
    ["Dashboard Auth", dashboard.auth_enabled ? "enabled" : "disabled"],
    ["Remote Dashboard", dashboard.allow_remote ? "enabled" : "disabled"],
    ["Max Body", limits.max_body_bytes || "n/a"],
    ["Rate Limit", limits.rate_limit || "off"],
    ["Read Timeout", timeouts.read || "n/a"],
    ["Write Timeout", timeouts.write || "n/a"],
    ["Idle Timeout", timeouts.idle || "n/a"],
    ["Access Log", logging.access_log ? "enabled" : "disabled"],
    ["Trace Dir", logging.trace_dir || "disabled"],
    ["TLS", tls.configured ? "configured" : "self-signed/default"],
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

function renderProbes(target, items) {
  if (!Array.isArray(items) || items.length === 0) {
    target.innerHTML = `<p class="empty">No probe results yet.</p>`;
    return;
  }
  target.innerHTML = `<div class="record-list">${items.slice(0, 5).map((item) => {
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
    const zeroRTTSupport = objectField(support, "0rtt");
    const migrationSupport = objectField(support, "migration");
    const otherSupport = Object.entries(support)
      .filter(([name]) => name !== "0rtt" && name !== "migration")
      .map(([name, entry]) => `${name}:${entry.coverage || "unknown"}`);
    const plan = objectField(analysis, "test_plan");
    const requested = arrayField(plan, "requested");
    const executed = arrayField(plan, "executed");
    const skipped = arrayField(plan, "skipped");
    const planPills = [
      requested.length ? pillMuted(`requested ${requested.join(",")}`) : "",
      executed.length ? pill(`executed ${executed.join(",")}`) : "",
      ...skipped.map((entry) => pillWarn(`skipped ${entry.name}`)),
    ].join("");
    const advancedPills = [
      zeroRTT.mode ? pillMuted(`0rtt ${zeroRTT.resumed ? "resumed" : "checked"}`) : "",
      typeof zeroRTT.time_saved_ms === "number" ? pill(`resume saved ${Number(zeroRTT.time_saved_ms).toFixed(2)}ms`) : "",
      migration.mode ? pillMuted(`migration ${migration.supported ? "reachable" : "checked"}`) : "",
      typeof migration.duration_ms === "number" ? pill(`migration ${Number(migration.duration_ms).toFixed(2)}ms`) : "",
      qpack.mode ? pillMuted(`qpack ${qpack.mode}`) : "",
      typeof qpack.estimated_ratio === "number" ? pill(`qpack ratio ${Number(qpack.estimated_ratio).toFixed(2)}`) : "",
      loss.mode ? pillMuted(`loss ${loss.signal || "checked"}`) : "",
      congestion.mode ? pillMuted(`congestion ${congestion.signal || "checked"}`) : "",
      version.mode ? pillMuted(`version ${version.alpn || version.observed_proto || "checked"}`) : "",
      retry.mode ? pillMuted(`retry ${retry.retry_observed ? "observed" : "not-seen"}`) : "",
      ecn.mode ? pillMuted(`ecn ${ecn.ecn_visible ? "visible" : "not-seen"}`) : "",
      spin.mode ? pillMuted(`spin ${spin.stability || "checked"}`) : "",
    ].join("");
    const supportPills = [
      zeroRTTSupport.coverage ? pillMuted(`0rtt ${zeroRTTSupport.coverage}`) : "",
      zeroRTTSupport.state ? pill(zeroRTTSupport.state) : "",
      migrationSupport.coverage ? pillMuted(`migration ${migrationSupport.coverage}`) : "",
      migrationSupport.state ? pill(migrationSupport.state) : "",
    ].join("");
    return `
      <article class="record">
        <h3>${escapeHTML(item.target || item.id || "probe")}</h3>
        <div class="metric-grid">
          ${metric("Status", item.status || "n/a")}
          ${metric("Proto", item.proto || "n/a")}
          ${metric("Total", item.duration || "n/a")}
          ${metric("P95", latency.p95 ? `${Number(latency.p95).toFixed(2)}ms` : "n/a")}
          ${metric("Streams", streams.attempted || 0)}
          ${metric("Bytes", response.body_bytes || 0)}
          ${metric("0-RTT", zeroRTT.mode ? (zeroRTT.resumed ? "resumed" : "checked") : "n/a")}
          ${metric("Migration", migration.mode ? (migration.supported ? "reachable" : "checked") : "n/a")}
          ${metric("Coverage", typeof supportSummary.coverage_ratio === "number" ? `${Math.round(supportSummary.coverage_ratio * 100)}%` : "n/a")}
        </div>
        <div class="pill-row">
          ${pill(`latency samples ${latency.samples || 0}`)}
          ${pill(`stream success ${Math.round((streams.success_rate || 0) * 100)}%`)}
          ${pill(`throughput ${Math.round(response.throughput_bytes_sec || 0)} B/s`)}
        </div>
        <div class="pill-row">${supportPills}</div>
        <div class="pill-row">${advancedPills}</div>
        <div class="pill-row">${planPills}</div>
        ${otherSupport.length ? `<p class="mini">Advanced support: ${escapeHTML(otherSupport.join(" | "))}</p>` : ""}
        ${supportSummary.requested_tests ? `<p class="mini">Support summary: requested ${supportSummary.requested_tests}, available ${supportSummary.available || 0}, not-run ${supportSummary.not_run || 0}, unavailable ${supportSummary.unavailable || 0}</p>` : ""}
        ${skipped.length ? `<p class="mini">Skipped: ${escapeHTML(skipped.map((entry) => `${entry.name}: ${entry.reason}`).join(" | "))}</p>` : ""}
        ${zeroRTT.note ? `<p class="mini">0-RTT: ${escapeHTML(String(zeroRTT.note))}</p>` : ""}
        ${migration.note ? `<p class="mini">Migration: ${escapeHTML(String(migration.note))}</p>` : ""}
        ${qpack.note ? `<p class="mini">QPACK: ${escapeHTML(String(qpack.note))}</p>` : ""}
        ${loss.note ? `<p class="mini">Loss: ${escapeHTML(String(loss.note))}</p>` : ""}
        ${congestion.note ? `<p class="mini">Congestion: ${escapeHTML(String(congestion.note))}</p>` : ""}
        ${version.note ? `<p class="mini">Version: ${escapeHTML(String(version.note))}</p>` : ""}
        ${retry.note ? `<p class="mini">Retry: ${escapeHTML(String(retry.note))}</p>` : ""}
        ${ecn.note ? `<p class="mini">ECN: ${escapeHTML(String(ecn.note))}</p>` : ""}
        ${spin.note ? `<p class="mini">Spin Bit: ${escapeHTML(String(spin.note))}</p>` : ""}
        ${zeroRTTSupport.summary ? `<p class="mini">0-RTT support: ${escapeHTML(String(zeroRTTSupport.summary))}</p>` : ""}
        ${migrationSupport.summary ? `<p class="mini">Migration support: ${escapeHTML(String(migrationSupport.summary))}</p>` : ""}
      </article>
    `;
  }).join("")}</div>`;
}

function renderBenches(target, items) {
  if (!Array.isArray(items) || items.length === 0) {
    target.innerHTML = `<p class="empty">No benchmark results yet.</p>`;
    return;
  }
  target.innerHTML = `<div class="record-list">${items.slice(0, 5).map((item) => {
    const protocols = benchProtocolsForItem(item);
    const summary = item.summary || {};
    const pills = protocols.map(([name, stats]) => {
      const p95 = stats && stats.latency_ms ? Number(stats.latency_ms.p95 || 0).toFixed(2) : "0.00";
      return pill(`${name} p95 ${p95}ms`);
    }).join("");
    const top = protocols[0] ? protocols[0][1] : {};
    const topErrorRate = top && typeof top.error_rate === "number" ? `${Math.round(top.error_rate * 100)}%` : "n/a";
    const topSamples = top && top.sampled_points ? top.sampled_points : 0;
    return `
      <article class="record">
        <h3>${escapeHTML(item.target || item.id || "bench")}</h3>
        <div class="metric-grid">
          ${metric("Duration", item.duration || "n/a")}
          ${metric("Concurrency", item.concurrency || 0)}
          ${metric("Protocols", protocols.length)}
          ${metric("Top Req/s", top.req_per_sec ? Number(top.req_per_sec).toFixed(2) : "n/a")}
          ${metric("Top Error", topErrorRate)}
          ${metric("Samples", topSamples)}
          ${metric("Healthy", summary.healthy_protocols || 0)}
          ${metric("Best", summary.best_protocol || "n/a")}
        </div>
        <div class="pill-row">${pills}</div>
        ${summary.protocols ? `<p class="mini">Bench summary: healthy ${summary.healthy_protocols || 0}, degraded ${summary.degraded_protocols || 0}, failed ${summary.failed_protocols || 0}, riskiest ${escapeHTML(String(summary.riskiest_protocol || "n/a"))}</p>` : ""}
      </article>
    `;
  }).join("")}</div>`;
}

function renderTraces(target, items) {
  if (!Array.isArray(items) || items.length === 0) {
    target.innerHTML = `<p class="empty">No trace files found.</p>`;
    return;
  }
  target.innerHTML = `<div class="record-list">${items.slice(0, 5).map((item) => `
    <article class="record">
      <h3>${escapeHTML(item.name || "trace")}</h3>
      <div class="metric-grid">
        ${metric("Size", `${item.size_bytes || 0} bytes`)}
        ${metric("Updated", item.modified_at || "n/a")}
      </div>
      <p class="mini">${escapeHTML(item.preview || "")}</p>
    </article>
  `).join("")}</div>`;
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

async function loadOverview() {
  const target = document.getElementById("overview");
  try {
    const [probesResponse, benchesResponse] = await Promise.all([
      fetch("/api/v1/probes"),
      fetch("/api/v1/benches"),
    ]);
    const probes = await probesResponse.json();
    const benches = await benchesResponse.json();
    renderOverview(target, probes, benches);
  } catch (error) {
    target.textContent = String(error);
  }
}

load("status", "/api/v1/status", renderStatus);
load("config", "/api/v1/config", renderConfig);
load("probes", "/api/v1/probes", renderProbes);
load("benches", "/api/v1/benches", renderBenches);
load("traces", "/api/v1/traces", renderTraces);
loadOverview();
