<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Transitional//EN" "http://www.w3.org/TR/xhtml1/DTD/xhtml1-transitional.dtd">
<!-- unifi-ap-bridge-webui v1.0.0 -->
<html xmlns="http://www.w3.org/1999/xhtml">
<head>
<meta http-equiv="X-UA-Compatible" content="IE=Edge"/>
<meta http-equiv="Content-Type" content="text/html; charset=utf-8" />
<meta http-equiv="Pragma" content="no-cache">
<meta http-equiv="Expires" content="-1">
<link rel="shortcut icon" href="images/favicon.png">
<link rel="icon" href="images/favicon.png">

<title>UniFi AP Bridge</title>

<link rel="stylesheet" type="text/css" href="/index_style.css">
<link rel="stylesheet" type="text/css" href="/form_style.css">

<style>
thead.collapsible-jquery {
  color: #fff;
  padding: 0;
  width: 100%;
  border: none;
  text-align: left;
  outline: none;
  cursor: pointer;
}
.SettingsTable {
  text-align: left;
}
.SettingsTable th {
  background-color: #1F2D35 !important;
  background: #2F3A3E !important;
  border-bottom: none !important;
  border-top: none !important;
  font-size: 12px !important;
  color: #fff !important;
  font-weight: bolder !important;
  padding: 4px !important;
}
.SettingsTable td.settingname {
  border-right: solid 1px #000;
  border-left: solid 1px #000;
  background-color: #1F2D35 !important;
  background: #2F3A3E !important;
  width: 35% !important;
  padding: 8px 10px;
  color: #fff;
  font-size: 12px;
}
.SettingsTable td.settingvalue {
  text-align: left !important;
  border-right: solid 1px #000;
  padding: 8px 10px;
  background-color: #475a5f;
  color: #ecf0f1;
  font-size: 12px;
}
.SettingsTable input, .SettingsTable select {
  vertical-align: middle;
}
.SettingsTable label {
  margin-right: 14px;
  cursor: pointer;
  vertical-align: middle;
}
.badge_online {
  background-color: #27ae60;
  color: #fff;
  padding: 3px 8px;
  border-radius: 3px;
  font-weight: bold;
  display: inline-block;
  font-size: 11px;
}
.badge_offline {
  background-color: #c0392b;
  color: #fff;
  padding: 3px 8px;
  border-radius: 3px;
  font-weight: bold;
  display: inline-block;
  font-size: 11px;
}
.stat_card_container {
  display: flex;
  gap: 12px;
  margin-top: 10px;
  margin-bottom: 12px;
}
.stat_card {
  flex: 1;
  background: #243342;
  border: 1px solid #3e4d66;
  border-radius: 4px;
  padding: 12px;
  text-align: center;
}
.stat_num {
  font-size: 26px;
  font-weight: bold;
  color: #2ecc71;
  margin-top: 3px;
}
.stat_label {
  font-size: 11px;
  color: #bdc3c7;
  text-transform: uppercase;
  letter-spacing: 0.5px;
}
.console_log {
  background-color: #12181f;
  color: #2ecc71;
  font-family: 'Courier New', Courier, monospace;
  font-size: 11px;
  padding: 10px;
  border-radius: 3px;
  border: 1px solid #34495e;
  height: 160px;
  overflow-y: auto;
  white-space: pre-wrap;
  margin-top: 8px;
  line-height: 1.4;
}
</style>

<script language="JavaScript" type="text/javascript" src="/js/jquery.js"></script>
<script language="JavaScript" type="text/javascript" src="/js/httpApi.js"></script>
<script language="JavaScript" type="text/javascript" src="/state.js"></script>
<script language="JavaScript" type="text/javascript" src="/general.js"></script>
<script language="JavaScript" type="text/javascript" src="/popup.js"></script>
<script language="JavaScript" type="text/javascript" src="/help.js"></script>
<script language="JavaScript" type="text/javascript" src="/validator.js"></script>

<script type="text/javascript">
var autoRefreshTimer = null;
var formInitialized = false;
var lastClusterHtml = "";
var detectedGatewayIP = "<% nvram_get("lan_gateway"); %>";

function validGatewayIP(value) {
  var parts = String(value || "").split(".");
  if (parts.length !== 4) return false;
  var octets = [];
  for (var i = 0; i < parts.length; i++) {
    if (!/^\d{1,3}$/.test(parts[i]) || (parts[i].length > 1 && parts[i].charAt(0) === "0")) return false;
    var octet = parseInt(parts[i], 10);
    if (octet > 255) return false;
    octets.push(octet);
  }
  return octets[0] >= 1 && octets[0] < 224 && octets[0] !== 127 && !(octets[0] === 255 && octets[1] === 255 && octets[2] === 255 && octets[3] === 255);
}

function detectedGatewayURL() {
  return validGatewayIP(detectedGatewayIP) ? "http://" + detectedGatewayIP + ":8080/inform" : "";
}

var updatePollingTimer = null;
var currentBranch = "main";
var installedVersion = "";
var activeUpdateRequest = null;
var activeActionRequest = null;
var actionPollingTimer = null;

function displayVersion(value) {
  if (!value || value === "-") return "Unknown";
  return "v" + String(value).replace(/^v/i, "");
}

function renderInstalledVersion() {
  document.getElementById("cur_ver_val").textContent = displayVersion(installedVersion) + " (" + currentBranch + ")";
}

function finishUpdateRequest() {
  if (updatePollingTimer !== null) clearInterval(updatePollingTimer);
  updatePollingTimer = null;
  activeUpdateRequest = null;
  document.getElementById("btn_check_update").disabled = false;
}

function finishActionRequest() {
  if (actionPollingTimer !== null) clearInterval(actionPollingTimer);
  actionPollingTimer = null;
  activeActionRequest = null;
  var applyButton = document.getElementById("btn_apply_settings");
  var syncButton = document.getElementById("btn_sync_cluster");
  if (applyButton) applyButton.disabled = false;
  if (syncButton) syncButton.disabled = false;
}

function actionMessage(message, failed) {
  var el = document.getElementById("action_status");
  if (!el) return;
  el.textContent = message;
  el.style.color = failed ? "#e74c3c" : "#bdc3c7";
}

function startActionRequest(operation, requestId, actionScript) {
  if (activeActionRequest || activeUpdateRequest) {
    actionMessage("Another bridge operation is still running. Wait for its result before starting another.", true);
    return false;
  }
  var request = {operation:operation, id:String(requestId), started:Date.now()};
  activeActionRequest = request;
  var applyButton = document.getElementById("btn_apply_settings");
  var syncButton = document.getElementById("btn_sync_cluster");
  if (applyButton) applyButton.disabled = true;
  if (syncButton) syncButton.disabled = true;
  document.form.action_script.value = actionScript;
  document.form.action_mode.value = "apply";
  document.form.action_wait.value = "0";
  document.form.current_page.value = "";
  document.form.next_page.value = "";
  actionMessage(operation === "sync" ? "Sync dispatched. Waiting for per-node verification…" : "Applying settings on this AP…", false);
  function fail(message) {
    if (activeActionRequest !== request) return;
    finishActionRequest();
    actionMessage(message, true);
  }
  submitUpdateAction(function() {
    if (activeActionRequest !== request) return;
    request.dispatchUnknown = true;
    actionMessage("Dispatch response was lost; checking this request ID. Do not resubmit while its outcome is unknown.", true);
  });
  if (activeActionRequest !== request) return false;
  actionPollingTimer = setInterval(function() {
    if (Date.now() - request.started > (operation === "sync" ? 180000 : 60000)) {
      fail(operation === "sync" ? "Completion was not confirmed before the deadline. Check each node's status before retrying; no request was resubmitted." : "Settings completion was not confirmed before the deadline. Refresh status before retrying; no request was resubmitted.");
      return;
    }
    if (operation === "settings") {
      loadActionStatus(function(st) {
        if (activeActionRequest !== request || !st || String(st.request_id || "") !== request.id || st.operation !== "settings") return;
        if (st.status === "SAVED" || st.status === "SAVED_NOOP") {
          finishActionRequest();
          actionMessage(st.message || "Settings saved locally; runtime and cluster outcomes remain unverified.", false);
          loadStatus();
        } else if (st.status === "START_FAILED_AFTER_SAVE") {
          fail(st.message || "Settings saved, but startup failed.");
        } else if (st.status === "REJECTED") {
          fail(st.message || "Settings could not be applied.");
        }
      });
    } else {
      loadOperationStatus(request, function(st) {
        if (activeActionRequest !== request || !st || String(st.request_id || "") !== request.id || st.operation !== "sync") return;
        if (st.nodes) updateClusterUI({nodes:st.nodes,last_sync:st.completed_at || "-"});
        if (/^(COMPLETED|PARTIAL|UNKNOWN|FAILED|REJECTED)$/.test(st.status)) {
          finishActionRequest();
          actionMessage(st.message || ("Sync " + st.status.toLowerCase() + ". Review each node's stage, freshness, version and commit below."), st.status === "FAILED" || st.status === "REJECTED" || st.status === "PARTIAL");
        }
      });
    }
  }, 1500);
  return true;
}

function loadActionStatus(done) {
  if (!activeActionRequest) return;
  var s = document.createElement("script");
  s.src = "/ext/unifi-bridge/action_status_" + activeActionRequest.id + ".js?_=" + Date.now();
  s.onload = function() {
    if (done) done(window.unifiBridgeAction || null);
    try { document.head.removeChild(s); } catch(e){}
  };
  s.onerror = function() { try { document.head.removeChild(s); } catch(e){} };
  document.head.appendChild(s);
}

function loadOperationStatus(request, done) {
  var s = document.createElement("script");
  s.src = "/ext/unifi-bridge/operation_" + request.id + ".js?_=" + Date.now();
  s.onload = function() {
    if (done) done(window.unifiBridgeOperation || null);
    try { document.head.removeChild(s); } catch(e){}
  };
  s.onerror = function() { try { document.head.removeChild(s); } catch(e){} };
  document.head.appendChild(s);
}

function newRequestId() {
  var suffix = String(Math.floor(Math.random() * 1000000));
  while (suffix.length < 6) suffix = "0" + suffix;
  return String(Date.now()) + suffix;
}

// POST through the normal Asuswrt dispatcher without executing its iframe HTML.
// This keeps firmware showLoading/Complete callbacks out of background actions.
function submitUpdateAction(onError) {
  var form = document.form;
  var parts = [];
  for (var i = 0; i < form.elements.length; i++) {
    var field = form.elements[i];
    if (!field.name || field.disabled || /^(button|submit|reset|file)$/i.test(field.type)) continue;
    if (/^(checkbox|radio)$/i.test(field.type) && !field.checked) continue;
    parts.push(encodeURIComponent(field.name) + "=" + encodeURIComponent(field.value));
  }
  var request = new XMLHttpRequest();
  request.open("POST", form.action, true);
  request.setRequestHeader("Content-Type", "application/x-www-form-urlencoded");
  request.timeout = 15000;
  request.onload = function() {
    if (request.status < 200 || request.status >= 300) onError();
  };
  request.onerror = onError;
  request.ontimeout = onError;
  request.send(parts.join("&"));
}

function startUpdateRequest(operation, branch) {
  if (activeActionRequest) {
    actionMessage("Another bridge operation is still running. Wait for its result before starting an update.", true);
    return;
  }
  finishUpdateRequest();
  var request = { id: newRequestId(), operation: operation, branch: branch, started: Date.now() };
  activeUpdateRequest = request;
  document.getElementById("btn_check_update").disabled = true;
  document.getElementById("btn_do_update").style.display = "none";
  document.form.action_script.value = "start_unifi_bridge_" + (operation === "check" ? "check_update_" : "do_update_") + branch + "_" + request.id;
  document.form.action_mode.value = "apply";
  document.form.action_wait.value = "0";
  document.form.current_page.value = "";
  document.form.next_page.value = "";
  document.form.unifibridge_branch.value = branch;
  document.getElementById("amng_custom").value = JSON.stringify({ action: operation === "check" ? "check_update" : "do_update", branch: branch, request_id: request.id });
  function fail(message) {
    if (activeUpdateRequest !== request) return;
    finishUpdateRequest();
    document.getElementById("update_badge").textContent = "REQUEST FAILED";
    document.getElementById("update_desc").textContent = message;
    document.getElementById("update_progress_box").style.display = "none";
  }
  submitUpdateAction(function() { fail("Could not contact the router. Retry the " + operation + " request."); });
  if (activeUpdateRequest !== request) return;
  updatePollingTimer = setInterval(function() {
    if (Date.now() - request.started > (operation === "check" ? 45000 : 300000)) {
      fail(operation === "check" ? "Update check timed out. Please retry." : "Update status timed out. Check bridge status before retrying installation.");
      return;
    }
    loadUpdateStatus();
    if (operation === "apply") loadStatus();
  }, 1500);
}


function SetCurrentPage() {
  document.form.next_page.value = "";
  document.form.current_page.value = "";
  window.refreshpage = function() {};
  try { window.parent.refreshpage = function() {}; } catch (e) {}
}

function initFormOnce(st) {
  if (formInitialized || !st) return;
  formInitialized = true;

  if (st.branch) {
    currentBranch = st.branch;
    var selB = document.getElementById("select_branch");
    if (selB) selB.value = st.branch;
    if (document.form.unifibridge_branch) document.form.unifibridge_branch.value = st.branch;
    var hdrT = document.getElementById("hdr_track");
    if (hdrT) {
      hdrT.textContent = st.branch === "dev" ? "Dev Track" : "Release Track";
      hdrT.style.backgroundColor = st.branch === "dev" ? "#d35400" : "#2980b9";
    }
  }

  if (typeof st.enabled === "boolean") {
    document.getElementById("unifibridge_enabled_1").checked = st.enabled;
    document.getElementById("unifibridge_enabled_0").checked = !st.enabled;
  }

  if (st.model_code) {
    var selM = document.getElementById("select_model");
    if (selM) selM.value = st.model_code;
    if (document.form.unifibridge_model) document.form.unifibridge_model.value = st.model_code;
  }

  if (st.interval_sec) {
    var selI = document.getElementById("select_interval");
    if (selI) selI.value = String(st.interval_sec);
    if (document.form.unifibridge_interval) document.form.unifibridge_interval.value = String(st.interval_sec);
  }

  if (typeof st.auto_gateway === "boolean" || st.controller_url) {
    var isAuto = typeof st.auto_gateway === "boolean" ? st.auto_gateway :
      (st.controller_url === ("http://" + detectedGatewayIP + ":8080/inform"));
    var rad1 = document.getElementById("unifibridge_auto_1");
    var rad0 = document.getElementById("unifibridge_auto_0");
    if (rad1 && rad0) {
      rad1.checked = isAuto;
      rad0.checked = !isAuto;
      toggleControllerInput();
      if (!isAuto) {
        document.getElementById("input_controller_url").value = st.controller_url || "";
      }
    }
  }
}

function escapeHTML(value) {
  return String(value === undefined || value === null ? "" : value).replace(/[&<>"']/g, function(c) {
    return {"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;","'":"&#39;"}[c];
  });
}

function displayMetric(value, maxValue) {
  var text = String(value == null ? "" : value);
  if (!/^\d+$/.test(text)) return "-";
  var number = Number(text);
  if (!isFinite(number) || (maxValue != null && number > maxValue)) return "-";
  return String(number);
}

function formatUptime(sec) {
  if (typeof sec !== "number" || !isFinite(sec) || sec <= 0) return "-";
  var days = Math.floor(sec / 86400);
  var hours = Math.floor((sec % 86400) / 3600);
  var mins = Math.floor((sec % 3600) / 60);
  if (days > 0) return days + "d " + hours + "h";
  if (hours > 0) return hours + "h " + mins + "m";
  return mins + "m";
}

function formatBytes(bytes) {
  if (typeof bytes !== "number" || !isFinite(bytes) || bytes <= 0) return "0 B";
  if (bytes >= 1099511627776) return (bytes / 1099511627776).toFixed(1) + " TB";
  if (bytes >= 1073741824) return (bytes / 1073741824).toFixed(1) + " GB";
  if (bytes >= 1048576) return (bytes / 1048576).toFixed(1) + " MB";
  if (bytes >= 1024) return (bytes / 1024).toFixed(1) + " KB";
  return bytes + " B";
}

function statusIsFresh(st) {
  var age = Date.now()/1000 - Number(st.status_updated_at);
  return Number.isFinite(age) && age >= -30 && age <= 3 * (Number(st.interval_sec) || 10) + 30;
}

function clusterState(n) {
  var label=n.sync_status || "Unknown";
  if (n.outcome === "failure" || /Failed|Unreachable|Stale|Stopped|No SSH/i.test(label)) return "error";
  if (n.enabled === false && n.running === false && n.freshness === "fresh") return n.pid ? "error" : "disabled";
  if (n.freshness === "stale" || n.freshness === "unknown" || n.status !== "RUNNING") return "unverified";
  if (label === "Local") return "local";
  // A version match is healthy sampling, not proof of configuration equality.
  if (/Unverified|Config Synced|Version Diff/i.test(label)) return "unverified";
  if (/Verified|In Sync/i.test(label)) return "verified";
  return "unverified";
}

// These views use only text nodes: station/SSID fields are untrusted data.
function diagnosticCellText(value) {
  return typeof value === "string" ? value.slice(0, 128) : "-";
}
function appendDiagnosticRow(body, values) {
  var row = document.createElement("tr");
  for (var i = 0; i < values.length; i++) {
    var cell = document.createElement("td");
    cell.style.padding = "6px";
    cell.textContent = String(values[i]);
    row.appendChild(cell);
  }
  body.appendChild(row);
}
function diagnosticNumber(value, suffix) {
  return typeof value === "number" && isFinite(value) ? String(Math.round(value * 10) / 10) + suffix : "-";
}
function updateDiagnosticsUI(st) {
  var radioBody = document.getElementById("rf_diagnostics_rows");
  if (radioBody) {
    radioBody.textContent = "";
    var radios = Array.isArray(st.radio_stats) ? st.radio_stats.slice(0, 8) : [];
    var trends = Array.isArray(st.passive_rf_trends) ? st.passive_rf_trends.slice(0, 8) : [];
    for (var i = 0; i < radios.length; i++) {
      var radio = radios[i] || {};
      var mean = null;
      for (var j = 0; j < trends.length; j++) if (trends[j] && trends[j].radio === radio.name) mean = trends[j].mean_busy_percent;
      appendDiagnosticRow(radioBody, [diagnosticCellText(radio.name), diagnosticNumber(radio.channel, ""),
        diagnosticNumber(radio.bw, " MHz"), diagnosticNumber(radio.cu_total, "%"),
        diagnosticNumber(radio.cu_obss, "%"), diagnosticNumber(radio.cu_nocat, "%"),
        diagnosticNumber(radio.cu_nopkt, "%"), diagnosticNumber(radio.cca_interference, "%"),
        diagnosticNumber(radio.noise_dbm, " dBm"),
        mean === null ? "Collecting samples" : diagnosticNumber(mean, "%")]);
    }
    if (!radios.length) appendDiagnosticRow(radioBody, ["Waiting for a successful radio sample"]);
  }
  var eventBody = document.getElementById("station_event_rows");
  if (eventBody) {
    eventBody.textContent = "";
    var events = Array.isArray(st.station_events) ? st.station_events.slice(-20).reverse() : [];
    var labels = {observed_association:"Joined (observed)", observed_disassociation:"Left (observed)",
      observed_radio_change:"Radio changed", observed_bssid_change:"BSSID changed"};
    for (var k = 0; k < events.length; k++) {
      var event = events[k] || {};
      var physical = diagnosticCellText(event.physical_radio || event.radio);
      if (event.previous_physical_radio) physical = diagnosticCellText(event.previous_physical_radio) + " → " + physical;
      appendDiagnosticRow(eventBody, [diagnosticCellText(event.observed_at), labels[event.type] || "Observed change",
        diagnosticCellText(event.mac), diagnosticCellText(event.essid), physical,
        event.signal_known === true ? diagnosticNumber(event.signal, " dBm") : "-",
        diagnosticNumber(event.poll_gap_seconds, " s")]);
    }
    if (!events.length) appendDiagnosticRow(eventBody, ["No changes observed since this daemon started"]);
  }
  var mapping = document.getElementById("wlan_mapping_status");
  if (mapping) {
    var info = st.wlan_mapping || {};
    mapping.textContent = diagnosticNumber(info.entries, " SSID mappings") +
      (info.file_enabled === true ? " · Automatic mapping file enabled" : " · Manual mapping") +
      (info.last_error ? " · " + diagnosticCellText(info.last_error) : "");
  }
  var commands = document.getElementById("controller_command_status");
  if (commands) {
    var commandInfo = st.controller_commands || {};
    var last = commandInfo.last_result || commandInfo.last_observation;
    commands.textContent = "UniFi reboot: " + (commandInfo.reboot_enabled === true ? "Enabled" : "Disabled") +
      (last ? " · Last command: " + diagnosticCellText(last.action) + (last.status ? " (" + diagnosticCellText(last.status) + ")" : " (observed only)") : " · No command observed");
  }
}

function updateUI(st) {
  if (!st) return;
  updateDiagnosticsUI(st);

  // Initialize form controls ONCE from daemon state so user edits are never overwritten
  initFormOnce(st);

  var badge = document.getElementById("bridge_badge");
  var statusText = document.getElementById("bridge_status_text");
  if (st.running && !statusIsFresh(st)) {
    badge.className = "badge_offline";
    badge.textContent = "STALE";
    statusText.textContent = "Telemetry is stale or the router clock differs; refresh and check the service.";
  } else if (st.running) {
    badge.className = "badge_online";
    badge.textContent = "RUNNING";
    statusText.textContent = "Daemon Active (PID " + (st.pid || "-") + ")" + (st.last_status && st.last_status.indexOf("Error:") === 0 ? " — " + st.last_status : "");
    if (st.sample_fresh === false) {
      statusText.textContent += " — Telemetry sample stale" + (Number(st.sample_age_sec) >= 0 ? " (" + st.sample_age_sec + "s old)" : " (no completed sample)");
    } else if (st.sample_stage_errors && Object.keys(st.sample_stage_errors).length > 0) {
      statusText.textContent += " — Partial telemetry; one or more sections are unavailable";
    }
  } else {
    badge.className = "badge_offline";
    badge.textContent = "STOPPED";
    statusText.textContent = "Daemon Inactive";
  }

  if (st.version) {
    var vText = displayVersion(st.version);
    var hdrV = document.getElementById("hdr_version");
    if (hdrV) hdrV.textContent = vText;
    var curV = document.getElementById("cur_ver_val");
    installedVersion = st.version;
    if (st.branch) currentBranch = st.branch;
    if (curV) renderInstalledVersion();
  }

  document.getElementById("model_val").textContent = (st.model || "UniFi 6 Mesh") + " (Firmware " + (st.firmware || "6.8.2") + ")";
  document.getElementById("controller_val").textContent = st.controller_url || detectedGatewayURL() || "Unknown";
  document.getElementById("heartbeat_val").textContent = (st.last_successful_inform || st.last_heartbeat || "-") + " [" + (st.mode || "GCM") + "]";

  document.getElementById("stat_total").textContent = st.total_stations !== undefined ? st.total_stations : "-";
  document.getElementById("stat_2g").textContent = st.stations_2g !== undefined ? st.stations_2g : "-";
  document.getElementById("stat_5g").textContent = st.stations_5g !== undefined ? st.stations_5g : "-";

  if (st.system_stats) {
    var sysLoadEl = document.getElementById("stat_sys_load");
    var sysSubEl = document.getElementById("stat_sys_sub");
    if (sysLoadEl && st.system_stats.cpu !== undefined && st.system_stats.cpu !== null) {
      sysLoadEl.textContent = st.system_stats.cpu + "% CPU";
    }
    if (sysLoadEl && (st.system_stats.cpu === undefined || st.system_stats.cpu === null)) sysLoadEl.textContent = "CPU: measuring...";
    if (sysSubEl && st.system_stats.cpu !== undefined && st.system_stats.cpu !== null && st.system_stats.mem !== undefined) {
      sysSubEl.textContent = "CPU: " + st.system_stats.cpu + "% | RAM: " + st.system_stats.mem + "%";
    }
  }

  var portsEl = document.getElementById("ports_val");
  if (portsEl && st.ports && st.ports.length > 0) {
    var portSignature = JSON.stringify(st.ports);
    if (portsEl._unifiBridgePorts !== portSignature) {
      var portWrap = document.createElement("div");
      portWrap.style.display = "flex";
      portWrap.style.flexWrap = "wrap";
      portWrap.style.gap = "6px";
      portWrap.style.marginTop = "2px";
      for (var p = 0; p < st.ports.length; p++) {
        var pt = st.ports[p] || {};
        var isUp = pt.up === true;
        var portName = typeof pt.name === "string" ? pt.name : "";
        if (!portName || portName.length > 64 || /[\u0000-\u001f\u007f]/.test(portName)) portName = "Port";
        var speed = pt.speed;
        var speedValid = typeof speed === "number" && isFinite(speed) && Math.floor(speed) === speed && speed > 0 && speed <= 100000;
        var state = isUp ? (speedValid ? String(speed) + "M " : "Speed unknown ") + (pt.is_uplink === true ? "UPLINK" : "FDX") : "DOWN";
        var badge = document.createElement("span");
        badge.style.background = isUp ? "#27ae60" : "#576574";
        badge.style.color = "#fff";
        badge.style.fontSize = "10px";
        badge.style.fontWeight = "bold";
        badge.style.padding = "2px 7px";
        badge.style.borderRadius = "3px";
        badge.textContent = portName + ": " + state;
        portWrap.appendChild(badge);
      }
      portsEl.textContent = "";
      portsEl.appendChild(portWrap);
      portsEl._unifiBridgePorts = portSignature;
    }
  }

  var elLoad2G = document.getElementById("load_2g");
  if (elLoad2G) {
    if (st.cu_total_2g !== undefined && st.cu_total_2g > 0) {
      elLoad2G.innerHTML = 'Channel Load: <strong style="color:#f39c12;">' + st.cu_total_2g + '%</strong>';
    } else {
      elLoad2G.textContent = 'Channel Load: -';
    }
  }

  var elLoad5G = document.getElementById("load_5g");
  if (elLoad5G) {
    if (st.cu_total_5g !== undefined && st.cu_total_5g > 0) {
      elLoad5G.innerHTML = 'Channel Load: <strong style="color:#3498db;">' + st.cu_total_5g + '%</strong>';
    } else {
      elLoad5G.textContent = 'Channel Load: -';
    }
  }

  if (st.recent_logs && st.recent_logs.length > 0) {
    var logBox = document.getElementById("console_box");
    var newLogText = st.recent_logs.join("\n");
    if (logBox.textContent !== newLogText) {
      var wasAtBottom = (logBox.scrollHeight - logBox.clientHeight <= logBox.scrollTop + 20);
      logBox.textContent = newLogText;
      if (wasAtBottom) {
        logBox.scrollTop = logBox.scrollHeight;
      }
    }
  }
}

function renderSatelliteNodesTelemetry(cl) {
  var container = document.getElementById("satellite_nodes_container");
  if (!container) return;
  if (!cl || !cl.nodes || cl.nodes.length <= 1) {
    container.style.display = "none";
    container.innerHTML = "";
    return;
  }

  var satNodes = [];
  for (var i = 0; i < cl.nodes.length; i++) {
    var nd = cl.nodes[i];
    if (nd.role && nd.role.indexOf("Main") === -1 && nd.sync_status !== "Local") {
      satNodes.push(nd);
    }
  }

  if (satNodes.length === 0) {
    container.style.display = "none";
    container.innerHTML = "";
    return;
  }

  var html = "";
  for (var s = 0; s < satNodes.length; s++) {
    var n = satNodes[s];
    var isRunning = n.status === "RUNNING";
    var sys = n.system_stats || {};
    var ports = n.ports || [];
    var radios = n.radio_stats || [];

    var cpuStr = typeof sys.cpu === "number" ? sys.cpu + "%" : "-";
    var memStr = typeof sys.mem === "number" ? sys.mem + "%" : "-";
    var upStr = typeof sys.uptime === "number" ? formatUptime(sys.uptime) : "-";
    var tempStr = n.temp || "-";
    var informStr = n.last_successful_inform ? escapeHTML(n.last_successful_inform) + " [" + escapeHTML(n.mode || "GCM") + "]" : "-";

    html += '<details open style="margin-bottom:12px; padding:10px; background:#222c31; border:1px solid #3e4d66; border-radius:6px;">';
    html += '<summary style="cursor:pointer; font-weight:bold; font-size:12px; color:#3498db; outline:none;">';
    html += 'Satellite Node Telemetry: <span style="color:#fff;">' + escapeHTML(n.name) + '</span> (' + escapeHTML(n.ip) + ')';
    html += isRunning
      ? ' <span class="badge_online" style="font-size:10px; margin-left:8px; background-color:#27ae60;">Active (' + escapeHTML(n.mode || "GCM") + ')</span>'
      : ' <span class="badge_offline" style="font-size:10px; margin-left:8px;">Offline</span>';
    html += '</summary>';

    html += '<div style="margin-top:10px;">';

    // System Summary Bar
    html += '<div style="display:flex; flex-wrap:wrap; gap:16px; padding:8px 12px; background:#2a363b; border-radius:4px; font-size:11px; color:#bdc3c7; margin-bottom:10px;">';
    html += '<div>Model: <strong style="color:#ecf0f1;">' + escapeHTML(n.model || "UniFi 6 In-Wall") + '</strong></div>';
    html += '<div>CPU: <strong style="color:#2ecc71;">' + escapeHTML(cpuStr) + '</strong></div>';
    html += '<div>RAM: <strong style="color:#f39c12;">' + escapeHTML(memStr) + '</strong></div>';
    html += '<div>Temp: <strong style="color:#e67e22;">' + escapeHTML(tempStr) + '</strong></div>';
    html += '<div>Uptime: <strong style="color:#ecf0f1;">' + escapeHTML(upStr) + '</strong></div>';
    html += '<div>Inform: <strong style="color:#3498db;">' + informStr + '</strong></div>';
    html += '</div>';

    // Switch Ports Table
    if (ports.length > 0) {
      html += '<div style="margin-bottom:10px;">';
      html += '<div style="font-size:11px; font-weight:bold; color:#ecf0f1; margin-bottom:4px;">Physical Switch Ports &amp; VLAN Connections:</div>';
      html += '<table width="100%" style="font-size:11px; text-align:left; border-collapse:collapse;">';
      html += '<thead><tr bgcolor="#1b2427" style="color:#bdc3c7;">';
      html += '<th style="padding:4px 8px; width:15%;">Port</th>';
      html += '<th style="padding:4px 8px; width:10%;">Interface</th>';
      html += '<th style="padding:4px 8px; width:12%;">Link</th>';
      html += '<th style="padding:4px 8px; width:15%;">Speed</th>';
      html += '<th style="padding:4px 8px; width:28%;">VLAN / Connected Device</th>';
      html += '<th style="padding:4px 8px; width:20%;">Traffic (RX / TX)</th>';
      html += '</tr></thead><tbody>';

      for (var p = 0; p < ports.length; p++) {
        var pt = ports[p] || {};
        var isUp = pt.up === true;
        var pName = typeof pt.name === "string" ? pt.name : "Port " + (pt.port_idx || (p + 1));
        var speedStr = isUp ? ((pt.speed || 0) > 0 ? pt.speed + " Mbps " + (pt.full_duplex ? "FDX" : "HDX") : "Connected") : "No Link";
        var linkBadge = isUp
          ? '<span class="badge_online" style="font-size:9px; background:#27ae60;">' + (pt.is_uplink ? "UPLINK" : "UP") + '</span>'
          : '<span style="font-size:9px; color:#7f8c8d;">DOWN</span>';

        var connInfo = "-";
        if (isUp) {
          if (pt.is_uplink) {
            var upTarget = pt.last_connection && pt.last_connection.ip ? pt.last_connection.ip : (pt.last_connection && pt.last_connection.mac ? pt.last_connection.mac : "Gateway");
            connInfo = '<span style="color:#3498db;">Uplink to ' + escapeHTML(String(upTarget)) + '</span>';
          } else if (pt.mac_table && pt.mac_table.length > 0) {
            var devList = [];
            for (var m = 0; m < pt.mac_table.length; m++) {
              var entry = pt.mac_table[m];
              var vlanBadge = entry.vlan ? '<span style="background:#8e44ad; color:#fff; font-size:9px; padding:1px 5px; border-radius:3px; font-weight:bold; margin-right:4px;">VLAN ' + entry.vlan + '</span>' : '';
              devList.push(vlanBadge + '<code style="color:#2ecc71;">' + escapeHTML(entry.mac || "") + '</code>');
            }
            connInfo = devList.join(", ");
          } else if (pt.last_connection && pt.last_connection.mac) {
            connInfo = '<code style="color:#2ecc71;">' + escapeHTML(pt.last_connection.mac) + '</code>';
          } else {
            connInfo = '<span style="color:#bdc3c7;">Connected (No MAC learned)</span>';
          }
        }

        var rxFormatted = formatBytes(pt.rx_bytes);
        var txFormatted = formatBytes(pt.tx_bytes);
        var trafficStr = isUp ? (rxFormatted + " / " + txFormatted) : "-";

        html += '<tr style="border-bottom:1px solid #2d3b40;">';
        html += '<td style="padding:4px 8px; font-weight:bold; color:#ecf0f1;">' + escapeHTML(pName) + '</td>';
        html += '<td style="padding:4px 8px; color:#bdc3c7;">' + escapeHTML(pt.ifname || "") + '</td>';
        html += '<td style="padding:4px 8px;">' + linkBadge + '</td>';
        html += '<td style="padding:4px 8px; color:' + (isUp ? "#2ecc71" : "#7f8c8d") + ';">' + escapeHTML(speedStr) + '</td>';
        html += '<td style="padding:4px 8px;">' + connInfo + '</td>';
        html += '<td style="padding:4px 8px; color:#bdc3c7; font-size:10px;">' + trafficStr + '</td>';
        html += '</tr>';
      }
      html += '</tbody></table></div>';
    }

    // Wireless Radios Table
    if (radios.length > 0) {
      html += '<div>';
      html += '<div style="font-size:11px; font-weight:bold; color:#ecf0f1; margin-bottom:4px;">Wireless Radios &amp; Frequency Channels:</div>';
      html += '<table width="100%" style="font-size:11px; text-align:left; border-collapse:collapse;">';
      html += '<thead><tr bgcolor="#1b2427" style="color:#bdc3c7;">';
      html += '<th style="padding:4px 8px; width:15%;">Radio</th>';
      html += '<th style="padding:4px 8px; width:15%;">Band</th>';
      html += '<th style="padding:4px 8px; width:12%;">Channel</th>';
      html += '<th style="padding:4px 8px; width:12%;">Width</th>';
      html += '<th style="padding:4px 8px; width:12%;">Stations</th>';
      html += '<th style="padding:4px 8px; width:12%;">Channel Load</th>';
      html += '<th style="padding:4px 8px; width:12%;">CCA Interf.</th>';
      html += '<th style="padding:4px 8px; width:10%;">Noise Floor</th>';
      html += '</tr></thead><tbody>';

      for (var r = 0; r < radios.length; r++) {
        var rd = radios[r] || {};
        var bandLabel = (rd.name === "wifi-ng" || rd.channel <= 14) ? "2.4 GHz" : ((rd.channel < 100) ? "5 GHz-1" : "5 GHz-2");
        var loadColor = (rd.cu_total || 0) > 70 ? "#e74c3c" : ((rd.cu_total || 0) > 40 ? "#f39c12" : "#2ecc71");
        var interfColor = (rd.cu_interf || 0) > 20 ? "#e74c3c" : "#bdc3c7";

        html += '<tr style="border-bottom:1px solid #2d3b40;">';
        html += '<td style="padding:4px 8px; font-weight:bold; color:#ecf0f1;">' + escapeHTML(rd.name || "") + '</td>';
        html += '<td style="padding:4px 8px; color:#bdc3c7;">' + escapeHTML(bandLabel) + '</td>';
        html += '<td style="padding:4px 8px; color:#3498db; font-weight:bold;">Ch ' + (rd.channel || "-") + '</td>';
        html += '<td style="padding:4px 8px; color:#bdc3c7;">' + (rd.bw ? rd.bw + " MHz" : "-") + '</td>';
        html += '<td style="padding:4px 8px; color:#2ecc71; font-weight:bold;">' + (rd.num_sta !== undefined ? rd.num_sta : "-") + '</td>';
        html += '<td style="padding:4px 8px; color:' + loadColor + '; font-weight:bold;">' + (rd.cu_total !== undefined ? rd.cu_total + "%" : "-") + '</td>';
        html += '<td style="padding:4px 8px; color:' + interfColor + ';">' + (rd.cu_interf !== undefined ? rd.cu_interf + "%" : "-") + '</td>';
        html += '<td style="padding:4px 8px; color:#bdc3c7;">' + (rd.noise_dbm ? rd.noise_dbm + " dBm" : "-") + '</td>';
        html += '</tr>';
      }
      html += '</tbody></table></div>';
    }

    html += '</div>';
    html += '</details>';
  }

  container.innerHTML = html;
  container.style.display = "block";
}

function updateClusterUI(cl) {
  if (!cl || !cl.nodes) return;
  var tbody = document.getElementById("cluster_nodes_tbody");
  if (!tbody) return;

  var html = "";
  var hasHealthError = false;
  var healthVerified = cl.nodes.length > 0;
  var allSync = cl.nodes.length > 0;
  var clusterTotal = 0;
  var cluster2g = 0;
  var cluster5g = 0;

  for (var i = 0; i < cl.nodes.length; i++) {
    var n = cl.nodes[i];
    var isLocal = n.role.indexOf("Main") !== -1;
    var state = clusterState(n);
    var isSync = state === "verified" || state === "local";
    if (!isSync) allSync = false;
    var nodeDisabledAndHealthy = n.enabled === false && n.running === false && n.freshness === "fresh" && !n.pid;
    var nodeRunningAndFresh = n.status === "RUNNING" && n.running !== false && n.freshness === "fresh";
    if (state === "error") hasHealthError = true;
    if (!nodeDisabledAndHealthy && !nodeRunningAndFresh) healthVerified = false;

    if (n.status === "RUNNING") {
      clusterTotal += Number(n.total_clients) || 0;
      cluster2g += Number(n.clients_2g) || 0;
      cluster5g += Number(n.clients_5g) || 0;
    }

    var statusBadge = n.enabled === false && n.running === false
      ? '<span style="font-size:10px;color:#bdc3c7;">DISABLED</span>'
      : (n.status === "RUNNING"
        ? '<span class="badge_online" style="font-size:10px;">RUNNING</span>'
        : '<span class="badge_offline" style="font-size:10px;">STOPPED</span>');

    var syncBadge = isSync
      ? '<span class="badge_online" style="font-size:10px; background-color:#27ae60;">' + escapeHTML(n.sync_status) + '</span>'
      : (state === 'error'
        ? '<span class="badge_offline" style="font-size:10px; background-color:#e74c3c;">' + escapeHTML(n.sync_status) + '</span>'
        : '<span class="badge_online" style="font-size:10px; background-color:#d35400;">' + escapeHTML(n.sync_status) + '</span>');

    html += '<tr style="border-bottom:1px solid #3e4d66;">';
    html += '<td class="settingvalue" style="font-weight:bold; color:#fff;">' + escapeHTML(n.name) + (isLocal ? ' <span style="font-size:10px; color:#f1c40f;">(This AP)</span>' : '') + '</td>';
    html += '<td class="settingvalue">' + escapeHTML(n.role) + '</td>';
    html += '<td class="settingvalue"><code style="color:#2ecc71;">' + escapeHTML(n.version || "Unknown") + '</code></td>';
    html += '<td class="settingvalue"><code style="font-size:10px;" title="' + escapeHTML(n.commit || "") + '">' + escapeHTML(n.commit ? String(n.commit).slice(0, 12) : "-") + '</code></td>';
    html += '<td class="settingvalue"><code>' + escapeHTML(n.ip) + '</code></td>';
    html += '<td class="settingvalue">' + statusBadge + '</td>';
    var clientsTotal = displayMetric(n.total_clients);
    var clients2g = displayMetric(n.clients_2g);
    var clients5g = displayMetric(n.clients_5g);
    var load2g = displayMetric(n.cu_2g, 100);
    var load5g = displayMetric(n.cu_5g, 100);
    var loadTag = (load2g !== "-" || load5g !== "-")
      ? ' <span style="font-size:10px; color:#f39c12;">[' + (load2g === "-" ? "0" : load2g) + '% / ' + (load5g === "-" ? "0" : load5g) + '% load]</span>' : '';
    html += '<td class="settingvalue">' + clientsTotal + ' <span style="font-size:10px; color:#bdc3c7;">(' + clients2g + 'G / ' + clients5g + 'A)</span>' + loadTag + '</td>';
    html += '<td class="settingvalue">' + syncBadge + '</td>';

    var fullTrace = (n.outcome || "unknown") + " · " + (n.stage || "unknown") + " · " + (n.freshness || "unknown") + " · dispatch " + (n.dispatch_result || "unknown");
    if (n.operation_id) fullTrace += " · op " + n.operation_id;
    if (n.requested_version) fullTrace += " · requested v" + String(n.requested_version).replace(/^v/i, "");
    if (n.requested_commit) fullTrace += " · requested " + String(n.requested_commit).slice(0, 12);
    if (n.commit) fullTrace += " · observed " + String(n.commit).slice(0, 12);
    if (n.enabled === false) fullTrace += " · disabled";
    if (n.last_successful_inform) fullTrace += " · inform " + n.last_successful_inform;

    var visualHealth = "";
    if (state === "error") {
      visualHealth = '<span class="badge_offline" style="font-size:9px; background:#c0392b;">ATTENTION</span>';
    } else if (n.status === "RUNNING") {
      var modeLabel = n.mode ? " (" + escapeHTML(n.mode) + ")" : "";
      visualHealth = '<span class="badge_online" style="font-size:9px; background:#27ae60;">ACTIVE' + modeLabel + '</span>';
    } else if (n.enabled === false) {
      visualHealth = '<span style="font-size:9px; color:#bdc3c7;">DISABLED</span>';
    } else {
      visualHealth = '<span class="badge_offline" style="font-size:9px;">STOPPED</span>';
    }

    var informPart = "";
    if (n.last_successful_inform) {
      var infParts = String(n.last_successful_inform).split(" ");
      var infTime = infParts.length > 1 ? infParts[1] : infParts[0];
      informPart = ' <span style="font-size:10px; color:#bdc3c7;" title="' + escapeHTML(n.last_successful_inform) + '">Inform: ' + escapeHTML(infTime) + '</span>';
    }

    var tempPart = (n.temp && n.temp !== "-") ? ' <span style="font-size:10px; color:#f39c12;">(' + escapeHTML(n.temp) + ')</span>' : "";
    var cpuPart = (n.system_stats && typeof n.system_stats.cpu === "number") ? ' <span style="font-size:10px; color:#9b59b6;">[' + n.system_stats.cpu + '% CPU]</span>' : "";

    html += '<td class="settingvalue" style="font-size:10px;" title="' + escapeHTML(fullTrace) + '">' + visualHealth + informPart + tempPart + cpuPart + '</td>';
    html += '</tr>';
  }

  // Only rewrite innerHTML if content has actually changed to prevent DOM reflow and scroll jumps
  if (html !== lastClusterHtml) {
    tbody.innerHTML = html;
    lastClusterHtml = html;
  }

  var syncBadgeEl = document.getElementById("cluster_sync_badge");
  if (syncBadgeEl) {
    if (hasHealthError) {
      syncBadgeEl.className = "badge_offline";
      syncBadgeEl.textContent = "HEALTH ATTENTION · CONFIGURATION UNVERIFIED";
      syncBadgeEl.style.backgroundColor = "#c0392b";
    } else if (allSync && healthVerified) {
      syncBadgeEl.className = "badge_online";
      syncBadgeEl.textContent = "HEALTH OK · CLUSTER IN SYNC";
      syncBadgeEl.style.backgroundColor = "#27ae60";
    } else {
      syncBadgeEl.className = "badge_online";
      syncBadgeEl.textContent = healthVerified ? "HEALTH OK · CONFIGURATION UNVERIFIED" : "CONFIGURATION UNVERIFIED";
      syncBadgeEl.style.backgroundColor = "#9a7200";
    }
  }

  var syncTimeEl = document.getElementById("last_sync_time");
  if (syncTimeEl) {
    syncTimeEl.textContent = cl.last_sync || "-";
  }

  if (cl.nodes.length > 1) {
    document.getElementById("stat_total").textContent = clusterTotal + " (Cluster)";
    document.getElementById("stat_2g").textContent = cluster2g;
    document.getElementById("stat_5g").textContent = cluster5g;
  }

  renderSatelliteNodesTelemetry(cl);
}

function updateUpdateUI(u) {
  if (!u) return;
  var pending = activeUpdateRequest;
  if (pending && (String(u.request_id || "") !== pending.id || u.branch !== pending.branch || u.operation !== pending.operation)) return;
  var badge = document.getElementById("update_badge");
  var btnDo = document.getElementById("btn_do_update");
  var desc = document.getElementById("update_desc");
  var remVal = document.getElementById("rem_ver_val");
  var curVal = document.getElementById("cur_ver_val");
  var pBox = document.getElementById("update_progress_box");

  if (!installedVersion && u.current_version && u.current_version !== "-") {
    installedVersion = u.current_version;
    if (u.installed_branch) currentBranch = u.installed_branch;
  }
  renderInstalledVersion();
  if (u.remote_version && u.remote_version !== "-") {
    var trackLabel = u.branch ? (" (" + u.branch + ")") : "";
    remVal.textContent = "Latest" + trackLabel + ": " + displayVersion(u.remote_version);
  }
  if (u.message) desc.textContent = u.message + (u.last_check ? " (Checked: " + u.last_check + ")" : "");

  if (u.status === "UPDATE_AVAILABLE") {
    badge.className = "badge_online";
    badge.style.backgroundColor = "#e67e22";
    badge.textContent = "UPDATE AVAILABLE";
    btnDo.style.display = "inline-block";
    btnDo.value = "Install " + displayVersion(u.remote_version) + " (" + u.branch + ")";
    pBox.style.display = "none";
  } else if (u.status === "UP_TO_DATE") {
    badge.className = "badge_online";
    badge.style.backgroundColor = "#27ae60";
    badge.textContent = u.installed_newer ? "INSTALLED IS NEWER" : "UP TO DATE";
    btnDo.style.display = "none";
    pBox.style.display = "none";
  } else if (u.status === "CHECKING") {
    badge.className = "badge_online";
    badge.style.backgroundColor = "#3498db";
    badge.textContent = "CHECKING...";
    btnDo.style.display = "none";
    pBox.style.display = "none";
  } else if (u.status === "UPDATING") {
    badge.className = "badge_online";
    badge.style.backgroundColor = "#f39c12";
    badge.textContent = "UPDATING...";
    btnDo.style.display = "none";
    pBox.style.display = "block";
  } else if (u.status === "SUCCESS") {
    badge.className = "badge_online";
    badge.style.backgroundColor = "#27ae60";
    badge.textContent = "UP TO DATE";
    btnDo.style.display = "none";
    pBox.style.display = "none";
    if (pending && pending.operation === "apply") {
      finishUpdateRequest();
      desc.textContent = "Update applied successfully! Refreshing page in 3 seconds...";
      setTimeout(function() { window.location.reload(); }, 3000);
    } else {
      desc.textContent = u.message || "UniFi AP Bridge is up to date.";
    }
  } else if (u.status === "ERROR") {
    badge.className = "badge_offline";
    badge.style.backgroundColor = "#c0392b";
    badge.textContent = "CHECK FAILED";
    btnDo.style.display = "none";
    pBox.style.display = "none";
  }
  if (pending && /^(UPDATE_AVAILABLE|UP_TO_DATE|SUCCESS|ERROR)$/.test(u.status)) finishUpdateRequest();
}

function loadUpdateStatus() {
  var s = document.createElement("script");
  s.src = "/ext/unifi-bridge/update_status.js?_=" + Date.now();
  s.onload = function() {
    if (window.unifiBridgeUpdate) {
      updateUpdateUI(window.unifiBridgeUpdate);
    }
    try { document.head.removeChild(s); } catch(e){}
  };
  document.head.appendChild(s);
}

function checkUpdateNow() {
  var selectedBranch = document.getElementById("select_branch").value;
  var badge = document.getElementById("update_badge");
  badge.className = "badge_online";
  badge.style.backgroundColor = "#3498db";
  badge.textContent = "CHECKING...";
  document.getElementById("update_progress_box").style.display = "none";
  document.getElementById("rem_ver_val").textContent = "Latest (" + selectedBranch + "): checking...";
  document.getElementById("update_desc").textContent = "Checking the " + selectedBranch + " track. This does not install software or change the installed track.";
  startUpdateRequest("check", selectedBranch);
}

function doUpdateNow() {
  var selectedBranch = document.getElementById("select_branch").value;
  if (!confirm("Install the latest UniFi AP Bridge build from the " + selectedBranch + " track across the cluster?\n\nThis restarts the bridge daemon.")) return;
  document.getElementById("update_progress_box").style.display = "block";
  var badge = document.getElementById("update_badge");
  badge.className = "badge_online";
  badge.style.backgroundColor = "#f39c12";
  badge.textContent = "UPDATING...";
  startUpdateRequest("apply", selectedBranch);
}

function onTrackChanged() {
  // The selector chooses what to check/install; the header reflects the running build.
  checkUpdateNow();
}

function loadCluster(done) {
  var s = document.createElement("script");
  s.src = "/ext/unifi-bridge/cluster.js?_=" + Date.now();
  s.onload = function() {
    if (window.unifiBridgeCluster) {
      updateClusterUI(window.unifiBridgeCluster);
      if (done) done(window.unifiBridgeCluster);
    } else if (done) {
      done(null);
    }
    try { document.head.removeChild(s); } catch(e){}
  };
  document.head.appendChild(s);
}

function loadStatus() {
  loadCluster();
  var s = document.createElement("script");
  s.src = "/ext/unifi-bridge/status.js?_=" + Date.now();
  s.onload = function() {
    if (window.unifiBridgeStatus) {
      updateUI(window.unifiBridgeStatus);
    }
    try { document.head.removeChild(s); } catch(e){}
  };
  s.onerror = function() {
    var badge = document.getElementById("bridge_badge");
    if (badge) {
      badge.className = "badge_offline";
      badge.textContent = "OFFLINE";
    }
  };
  document.head.appendChild(s);
}

function toggleAutoRefresh(cb) {
  if (cb.checked) {
    if (!autoRefreshTimer) autoRefreshTimer = setInterval(loadStatus, 15000);
  } else {
    if (autoRefreshTimer) {
      clearInterval(autoRefreshTimer);
      autoRefreshTimer = null;
    }
  }
}

function toggleControllerInput() {
  var isAuto = document.getElementById("unifibridge_auto_1").checked;
  var customDiv = document.getElementById("custom_url_div");
  if (isAuto) {
    customDiv.style.display = "none";
    document.getElementById("input_controller_url").value = detectedGatewayURL();
  } else {
    customDiv.style.display = "block";
  }
}

function syncClusterNow() {
  var requestId = newRequestId();
  document.form.current_page.value = "";
  document.form.next_page.value = "";
  window.refreshpage = function() {};
  try { window.parent.refreshpage = function() {}; } catch (e) {}

  startActionRequest("sync", requestId, "start_unifi_bridge_sync_" + requestId);
}

function applySettings() {
  var isEnabled = document.getElementById("unifibridge_enabled_1").checked ? "1" : "0";
  var isAutoGw = document.getElementById("unifibridge_auto_1").checked ? "1" : "0";
  var ctrlUrl = document.getElementById("input_controller_url").value.trim();
  var model = document.getElementById("select_model").value;
  var interval = document.getElementById("select_interval").value;
  var requestId = newRequestId();
  // The comparison selector does not change the installed track.
  var branch = currentBranch;

  if (isAutoGw === "1") {
    ctrlUrl = detectedGatewayURL();
  }

  document.form.current_page.value = "";
  document.form.next_page.value = "";
  window.refreshpage = function() {};
  try { window.parent.refreshpage = function() {}; } catch (e) {}

  document.form.unifibridge_enabled.value = isEnabled;
  document.form.unifibridge_auto_gateway.value = isAutoGw;
  document.form.unifibridge_controller_url.value = ctrlUrl;
  document.form.unifibridge_model.value = model;
  document.form.unifibridge_interval.value = interval;
  document.form.unifibridge_branch.value = branch;

  var amng = document.getElementById("amng_custom");
  if (amng) {
    var settingsPatch = JSON.stringify({
      enabled: isEnabled === "1",
      auto_gateway: isAutoGw === "1",
      controller_url: ctrlUrl,
      model: model,
      interval: parseInt(interval, 10),
      branch: branch,
      request_id: requestId
    });
    // Asuswrt-Merlin persists namespaced custom settings as lines in
    // custom_settings.txt instead of retaining arbitrary amng_custom JSON.
    amng.value = JSON.stringify({unifibridge_payload: settingsPatch});
  }

  startActionRequest("settings", requestId, "start_unifi_bridge_apply_" + requestId);
}

function initial(){
  SetCurrentPage();
  if (typeof show_menu === "function") {
    show_menu();
  }
  document.getElementById("detected_gw_text").textContent = validGatewayIP(detectedGatewayIP) ? detectedGatewayIP : "not detected";
  loadStatus();
  loadUpdateStatus();
  // Auto-refresh is disabled by default to ensure seamless form editing and scrolling without layout thrashing
}
</script>
</head>

<body onload="initial();">
<div id="TopBanner"></div>
<div id="Loading" class="popup_bg"></div>

<iframe name="hidden_frame" id="hidden_frame" src="about:blank" width="0" height="0" frameborder="0"></iframe>

<form method="post" name="form" id="ruleForm" action="/start_apply.htm" target="hidden_frame">
<input type="hidden" name="action_script" value="start_unifi_bridge_apply">
<input type="hidden" name="current_page" value="">
<input type="hidden" name="next_page" value="">
<input type="hidden" name="modified" value="0">
<input type="hidden" name="action_mode" value="apply">
<input type="hidden" name="action_wait" value="3">
<input type="hidden" name="first_time" value="">
<input type="hidden" name="SystemCmd" value="">
<input type="hidden" name="preferred_lang" id="preferred_lang" value="<% nvram_get("preferred_lang"); %>">
<input type="hidden" name="firmver" value="<% nvram_get("firmver"); %>">

<!-- Custom Plugin Settings -->
<input type="hidden" name="amng_custom" id="amng_custom" value="">
<input type="hidden" name="unifibridge_enabled" value="1">
<input type="hidden" name="unifibridge_auto_gateway" value="1">
<input type="hidden" name="unifibridge_controller_url" value="">
<input type="hidden" name="unifibridge_model" value="U6M">
<input type="hidden" name="unifibridge_interval" value="10">
<input type="hidden" name="unifibridge_branch" value="main">

<table class="content" align="center" cellpadding="0" cellspacing="0">
  <tr>
    <td width="17">&nbsp;</td>
    <td valign="top" width="202">
      <div id="mainMenu"></div>
      <div id="subMenu"></div>
    </td>
    <td valign="top">
      <div id="tabMenu" class="submenuBlock"></div>
      <table width="98%" border="0" align="left" cellpadding="0" cellspacing="0">
        <tr>
          <td valign="top">
            <table width="760px" border="0" cellpadding="4" cellspacing="0" bordercolor="#6b8fa3" class="FormTitle" id="FormTitle">
              <tbody>
                <tr bgcolor="#4D595D">
                  <td valign="top">
                    <div>&nbsp;</div>
                    <div class="formfonttitle" id="scripttitle" style="text-align:center;">
                      UniFi AP Bridge
                      <span id="hdr_version" style="font-size:12px; font-weight:normal; background:#34495e; color:#ecf0f1; padding:3px 8px; border-radius:3px; vertical-align:middle; margin-left:6px;">v1.0.0</span>
                      <span id="hdr_track" style="font-size:11px; font-weight:normal; background:#2980b9; color:#fff; padding:3px 6px; border-radius:3px; vertical-align:middle; margin-left:4px;">Release Track</span>
                    </div>
                    <div style="margin:10px 0 10px 5px;" class="splitLine"></div>
                    <div class="formfontdesc">Emulate a native UniFi 6 Access Point on Asuswrt-Merlin, feeding client associations, Wi-Fi 6 telemetry, and live signal metrics directly into your UniFi Network Controller.</div>

                    <!-- Metric Cards -->
                    <div class="stat_card_container">
                      <div class="stat_card">
                        <div class="stat_label">Total Wireless Clients</div>
                        <div class="stat_num" id="stat_total">-</div>
                      </div>
                      <div class="stat_card">
                        <div class="stat_label">2.4 GHz (IoT / Prime)</div>
                        <div class="stat_num" style="color:#f39c12;" id="stat_2g">-</div>
                        <div class="stat_sub" id="load_2g" style="font-size:11px; color:#bdc3c7; margin-top:4px;">Channel Load: -</div>
                      </div>
                      <div class="stat_card">
                        <div class="stat_label">5 GHz (High-Speed)</div>
                        <div class="stat_num" style="color:#3498db;" id="stat_5g">-</div>
                        <div class="stat_sub" id="load_5g" style="font-size:11px; color:#bdc3c7; margin-top:4px;">Channel Load: -</div>
                      </div>
                      <div class="stat_card">
                        <div class="stat_label">Router Resources</div>
                        <div class="stat_num" style="color:#9b59b6;" id="stat_sys_load">-</div>
                        <div class="stat_sub" id="stat_sys_sub" style="font-size:11px; color:#bdc3c7; margin-top:4px;">CPU: - | RAM: -</div>
                      </div>
                    </div>

                    <details style="margin-bottom:12px; padding:10px; background:#263238; border-radius:6px;">
                      <summary style="cursor:pointer; font-weight:bold;">RF diagnostics &amp; observed client changes</summary>
                      <p style="color:#bdc3c7; font-size:11px;">Passive serving-radio measurements. Other WiFi is traffic from neighboring WiFi networks. Bad-frame/control activity includes damaged frames and some control traffic; noise-like activity and the driver-reported non-WiFi CCA interference sample are separate measurements. CCA sample age is unknown. These measurements are not additive and do not identify the source of interference. This CCA value is not UniFi Avg. Interference; equivalence is unproven. A 0% UniFi value does not mean no interference. Missing measurements appear as dashes. Trends cover recent successful samples.</p>
                      <table width="100%" style="font-size:11px; text-align:left;">
                        <thead><tr><th>Radio</th><th>Channel</th><th>Actual width</th><th>Busy</th><th>Other WiFi (OBSS)</th><th>Bad frames / control</th><th>Noise-like activity</th><th>Non-WiFi CCA interference (age unknown)</th><th>Noise floor</th><th>Mean busy</th></tr></thead>
                        <tbody id="rf_diagnostics_rows"></tbody>
                      </table>
                      <p style="color:#bdc3c7; font-size:11px;">AP-local observations, held in RAM. Times and gaps reflect polling; departures do not identify the next AP or a disconnect reason.</p>
                      <table width="100%" style="font-size:11px; text-align:left;">
                        <thead><tr><th>Observed at</th><th>Change</th><th>Client</th><th>SSID</th><th>Radio</th><th>Signal</th><th>Poll gap</th></tr></thead>
                        <tbody id="station_event_rows"></tbody>
                      </table>
                      <p id="wlan_mapping_status" style="color:#bdc3c7; font-size:11px;"></p>
                      <p id="controller_command_status" style="color:#bdc3c7; font-size:11px;"></p>
                    </details>

                    <!-- Cluster Nodes & Synchronization Table -->
                    <table width="100%" border="1" align="center" cellpadding="2" cellspacing="0" bordercolor="#6b8fa3" class="FormTable SettingsTable" style="border:0px; margin-bottom:12px;" id="table_cluster">
                      <thead class="collapsible-jquery">
                        <tr>
                          <td colspan="9">
                            <div style="display:flex; justify-content:space-between; align-items:center;">
                              <span>Cluster Access Points & Synchronization Status</span>
                          <span id="cluster_sync_badge" class="badge_online" style="font-size:10px; margin-right:5px; background-color:#9a7200;">CONFIGURATION UNVERIFIED</span>
                            </div>
                          </td>
                        </tr>
                      </thead>
                      <thead>
                        <tr bgcolor="#2F3A3E" style="color:#bdc3c7; font-size:11px; text-align:left;">
                          <th style="padding:6px 10px; width:20%;">Access Point</th>
                          <th style="padding:6px 10px; width:14%;">Role</th>
                          <th style="padding:6px 10px; width:11%;">Version</th>
                          <th style="padding:6px 10px; width:10%;">Commit</th>
                          <th style="padding:6px 10px; width:14%;">IP Address</th>
                          <th style="padding:6px 10px; width:13%;">Status</th>
                          <th style="padding:6px 10px; width:14%;">Clients</th>
                          <th style="padding:6px 10px; width:14%;">Sync Status</th>
                          <th style="padding:6px 10px; width:22%;">Operation / Health</th>
                        </tr>
                      </thead>
                      <tbody id="cluster_nodes_tbody">
                        <tr>
                          <td colspan="9" style="padding:10px; text-align:center; color:#bdc3c7; background-color:#475a5f;">Loading cluster node status...</td>
                        </tr>
                      </tbody>
                    </table>
                    <div style="display:flex; justify-content:space-between; align-items:center; margin:-6px 0 4px 2px; font-size:11px; color:#bdc3c7;">
                      <span>Last Cluster Sync: <span id="last_sync_time" style="color:#ecf0f1; font-weight:bold;">-</span></span>
                      <input id="btn_sync_cluster" class="button_gen" type="button" value="Sync Cluster Now" onclick="syncClusterNow();" style="padding:2px 10px; font-size:11px;" />
                    </div>
                    <div id="action_status" role="status" aria-live="polite" style="min-height:16px; margin:0 0 14px 2px; font-size:11px; color:#bdc3c7;"></div>

                    <!-- Satellite Node Hardware & Ports Telemetry -->
                    <div id="satellite_nodes_container" style="display:none; margin-bottom:14px;"></div>

                    <!-- Software Updates & Track Management Table -->
                    <table width="100%" border="1" align="center" cellpadding="2" cellspacing="0" bordercolor="#6b8fa3" class="FormTable SettingsTable" style="border:0px; margin-bottom:12px;" id="table_updates">
                      <thead class="collapsible-jquery">
                        <tr>
                          <td colspan="2">
                            <div style="display:flex; justify-content:space-between; align-items:center;">
                              <span>Software Updates & Release Track Management</span>
                              <span id="update_badge" class="badge_online" style="font-size:10px; margin-right:5px; background-color:#7f8c8d;">READY</span>
                            </div>
                          </td>
                        </tr>
                      </thead>
                      <tr>
                        <td class="settingname">
                          Release Track / Branch<br/>
                          <span style="font-size:10px;color:#bdc3c7;">Switch between official releases and dev builds</span>
                        </td>
                        <td class="settingvalue">
                          <select class="input_option" id="select_branch" onchange="onTrackChanged();">
                            <option value="main" selected>Official Release (main)</option>
                            <option value="dev">Development Builds (dev)</option>
                          </select>
                          <span style="font-size:11px; color:#bdc3c7; margin-left:10px;">Select track to check or install</span>
                        </td>
                      </tr>
                      <tr>
                        <td class="settingname">
                          Current & Remote Version<br/>
                          <span style="font-size:10px;color:#bdc3c7;">Live version comparison with GitHub</span>
                        </td>
                        <td class="settingvalue">
                          <div style="display:flex; align-items:center; gap:12px;">
                            <div>Installed: <span id="cur_ver_val" style="font-weight:bold; color:#2ecc71;">Unknown</span></div>
                            <div id="rem_ver_val" style="color:#bdc3c7;">-</div>
                          </div>
                          <div id="update_desc" style="font-size:11px; color:#bdc3c7; margin-top:5px;">Click "Check for Updates" to query the repository.</div>
                          <div id="update_progress_box" style="display:none; margin-top:8px; padding:8px 12px; background:#243342; border:1px solid #3498db; border-radius:4px; color:#3498db; font-size:11px;">
                            Updating UniFi AP Bridge daemon & propagating across cluster nodes... Please wait (do not navigate away).
                          </div>
                        </td>
                      </tr>
                      <tr>
                        <td class="settingname">
                          Update Action<br/>
                          <span style="font-size:10px;color:#bdc3c7;">Query repository or apply update</span>
                        </td>
                        <td class="settingvalue">
                          <input class="button_gen" type="button" id="btn_check_update" value="Check for Updates" onclick="checkUpdateNow();" />
                          <input class="button_gen" type="button" id="btn_do_update" value="Update Now" onclick="doUpdateNow();" style="display:none; margin-left:10px; background-color:#e67e22; color:#fff; font-weight:bold;" />
                        </td>
                      </tr>
                    </table>

                    <!-- Status & Telemetry Table -->
                    <table width="100%" border="1" align="center" cellpadding="2" cellspacing="0" bordercolor="#6b8fa3" class="FormTable SettingsTable" style="border:0px;" id="table_status">
                      <thead class="collapsible-jquery">
                        <tr><td colspan="2">Status & Telemetry</td></tr>
                      </thead>
                      <tr>
                        <td class="settingname">Service Status</td>
                        <td class="settingvalue">
                          <span id="bridge_badge" class="badge_online">RUNNING</span>
                          <span id="bridge_status_text" style="margin-left:6px;">(Daemon Active)</span>
                        </td>
                      </tr>
                      <tr>
                        <td class="settingname">Emulated AP Model</td>
                        <td class="settingvalue" id="model_val">UniFi 6 Mesh (U6M) - FW 6.8.2.15592</td>
                      </tr>
                      <tr>
                        <td class="settingname">UniFi Controller Inform URL</td>
                        <td class="settingvalue"><code id="controller_val">Unknown</code></td>
                      </tr>
                      <tr>
                        <td class="settingname">Last Heartbeat Exchange</td>
                        <td class="settingvalue" id="heartbeat_val">-</td>
                      </tr>
                      <tr>
                        <td class="settingname">Physical Switch Ports</td>
                        <td class="settingvalue" id="ports_val">-</td>
                      </tr>
                    </table>

                    <div style="line-height:10px;">&nbsp;</div>

                    <!-- Configuration Table -->
                    <table width="100%" border="1" align="center" cellpadding="2" cellspacing="0" bordercolor="#6b8fa3" class="FormTable SettingsTable" style="border:0px;" id="table_config">
                      <thead class="collapsible-jquery">
                        <tr>
                          <td colspan="2">
                            <span>Configuration</span>
                            <span style="font-size:11px; color:#bdc3c7; font-weight:normal; margin-left:10px;">(Settings apply locally; cluster outcomes are reported per node)</span>
                          </td>
                        </tr>
                      </thead>
                      <tr>
                        <td class="settingname">
                          Enable UniFi AP Bridge<br/>
                          <span style="font-size:10px;color:#bdc3c7;">Start the background telemetry daemon</span>
                        </td>
                        <td class="settingvalue">
                          <label><input type="radio" name="radio_enabled" id="unifibridge_enabled_1" value="1" checked> Yes</label>
                          <label><input type="radio" name="radio_enabled" id="unifibridge_enabled_0" value="0"> No</label>
                        </td>
                      </tr>
                      <tr>
                        <td class="settingname">
                          Controller Discovery Mode<br/>
                          <span style="font-size:10px;color:#bdc3c7;">Select upstream gateway or custom URL</span>
                        </td>
                        <td class="settingvalue">
                          <div>
                            <label><input type="radio" name="radio_auto" id="unifibridge_auto_1" value="1" checked onchange="toggleControllerInput();"> Auto-detect Gateway (<code>http://<span id="detected_gw_text">not detected</span>:8080/inform</code>)</label>
                          </div>
                          <div style="margin-top:6px;">
                            <label><input type="radio" name="radio_auto" id="unifibridge_auto_0" value="0" onchange="toggleControllerInput();"> Custom Inform URL</label>
                          </div>
                          <div id="custom_url_div" style="display:none;margin-top:6px;padding-left:22px;">
                            <input type="text" class="input_32_table" id="input_controller_url" value="" maxlength="128" placeholder="http://ip-or-host:8080/inform">
                          </div>
                        </td>
                      </tr>
                      <tr>
                        <td class="settingname">
                          Emulated AP Hardware Profile<br/>
                          <span style="font-size:10px;color:#bdc3c7;">UniFi hardware model reported to controller</span>
                        </td>
                        <td class="settingvalue">
                          <select class="input_option" id="select_model" onchange="this.dataset.userModified='1';">
                            <option value="U6M">UniFi 6 Mesh (U6M) - Dual/Tri-Band Wi-Fi 6</option>
                            <option value="U6IW" selected>UniFi 6 In-Wall (U6-IW) - 4-Port Switch & Port Manager (Recommended)</option>
                            <option value="U6P">UniFi 6 Pro (U6P) - Wi-Fi 6 4x4 MU-MIMO</option>
                            <option value="U6L">UniFi 6 Lite (U6L) - Wi-Fi 6 2x2</option>
                          </select>
                        </td>
                      </tr>
                      <tr>
                        <td class="settingname">
                          Heartbeat Reporting Interval<br/>
                          <span style="font-size:10px;color:#bdc3c7;">Inform exchange frequency with controller</span>
                        </td>
                        <td class="settingvalue">
                          <select class="input_option" id="select_interval">
                            <option value="10" selected>10 seconds (Recommended)</option>
                            <option value="15">15 seconds</option>
                            <option value="30">30 seconds</option>
                            <option value="60">60 seconds</option>
                          </select>
                        </td>
                      </tr>
                    </table>

                    <!-- Apply Button -->
                    <div class="apply_gen" style="margin-top:15px;text-align:center;">
                      <input id="btn_apply_settings" class="button_gen" type="button" value="Apply Settings" onclick="applySettings();" />
                    </div>

                    <!-- Live Heartbeat Events -->
                    <div style="margin-top:18px;">
                      <div style="display:flex; justify-content:space-between; align-items:center;">
                        <span style="font-size:12px; font-weight:bold; color:#ecf0f1;">Live Heartbeat Events</span>
                        <div>
                          <label style="font-size:11px; color:#bdc3c7; cursor:pointer;">
                            <input type="checkbox" id="auto_refresh_chk" onclick="toggleAutoRefresh(this);"> Auto-refresh (15s)
                          </label>
                          <input type="button" class="button_gen" style="margin-left:8px; padding:2px 8px; font-size:11px;" value="Refresh" onclick="loadStatus();">
                        </div>
                      </div>
                      <div id="console_box" class="console_log">Loading heartbeat telemetry...</div>
                    </div>

                  </td>
                </tr>
              </tbody>
            </table>
          </td>
        </tr>
      </table>
    </td>
    <td width="10" align="center" valign="top">&nbsp;</td>
  </tr>
</table>
</form>
<div id="footer"></div>
</body>
</html>
