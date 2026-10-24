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
var detectedGatewayIP = "<% nvram_get("lan_gateway"); %>" || "10.100.1.1";

function SetCurrentPage() {
  document.form.next_page.value = window.location.pathname.substring(1);
  document.form.current_page.value = window.location.pathname.substring(1);
}

function updateUI(st) {
  if (!st) return;
  var badge = document.getElementById("bridge_badge");
  var statusText = document.getElementById("bridge_status_text");
  if (st.running) {
    badge.className = "badge_online";
    badge.textContent = "RUNNING";
    statusText.textContent = "Daemon Active (PID " + (st.pid || "-") + ")";
  } else {
    badge.className = "badge_offline";
    badge.textContent = "STOPPED";
    statusText.textContent = "Daemon Inactive";
  }

  document.getElementById("model_val").textContent = (st.model || "UniFi 6 Mesh") + " (Firmware " + (st.firmware || "6.8.2") + ")";
  document.getElementById("controller_val").textContent = st.controller_url || ("http://" + detectedGatewayIP + ":8080/inform");
  document.getElementById("heartbeat_val").textContent = (st.last_heartbeat || "-") + " [" + (st.mode || "GCM") + "]";

  document.getElementById("stat_total").textContent = st.total_stations !== undefined ? st.total_stations : "-";
  document.getElementById("stat_2g").textContent = st.stations_2g !== undefined ? st.stations_2g : "-";
  document.getElementById("stat_5g").textContent = st.stations_5g !== undefined ? st.stations_5g : "-";

  if (st.recent_logs && st.recent_logs.length > 0) {
    var logBox = document.getElementById("console_box");
    logBox.textContent = st.recent_logs.join("\n");
    logBox.scrollTop = logBox.scrollHeight;
  }
}

function updateClusterUI(cl) {
  if (!cl || !cl.nodes) return;
  var tbody = document.getElementById("cluster_nodes_tbody");
  if (!tbody) return;

  var html = "";
  var allInSync = true;
  var clusterTotal = 0;
  var cluster2g = 0;
  var cluster5g = 0;

  for (var i = 0; i < cl.nodes.length; i++) {
    var n = cl.nodes[i];
    var isLocal = n.role.indexOf("Main") !== -1;
    var isSync = n.sync_status.indexOf("In Sync") !== -1;
    if (!isSync) allInSync = false;

    clusterTotal += (n.total_clients || 0);
    cluster2g += (n.clients_2g || 0);
    cluster5g += (n.clients_5g || 0);

    var statusBadge = n.status === "RUNNING"
      ? '<span class="badge_online" style="font-size:10px;">RUNNING</span>'
      : '<span class="badge_offline" style="font-size:10px;">STOPPED</span>';

    var syncBadge = isSync
      ? '<span class="badge_online" style="font-size:10px; background-color:#2980b9;">' + n.sync_status + '</span>'
      : '<span class="badge_offline" style="font-size:10px;">' + n.sync_status + '</span>';

    html += '<tr style="border-bottom:1px solid #3e4d66;">';
    html += '<td class="settingvalue" style="font-weight:bold; color:#fff;">' + n.name + (isLocal ? ' <span style="font-size:10px; color:#f1c40f;">(This AP)</span>' : '') + '</td>';
    html += '<td class="settingvalue">' + n.role + '</td>';
    html += '<td class="settingvalue"><code>' + n.ip + '</code></td>';
    html += '<td class="settingvalue">' + statusBadge + '</td>';
    html += '<td class="settingvalue">' + n.total_clients + ' <span style="font-size:10px; color:#bdc3c7;">(' + n.clients_2g + 'G / ' + n.clients_5g + 'A)</span></td>';
    html += '<td class="settingvalue">' + syncBadge + '</td>';
    html += '</tr>';
  }

  tbody.innerHTML = html;

  var syncBadgeEl = document.getElementById("cluster_sync_badge");
  if (syncBadgeEl) {
    if (allInSync) {
      syncBadgeEl.className = "badge_online";
      syncBadgeEl.textContent = "ALL NODES IN SYNC";
    } else {
      syncBadgeEl.className = "badge_offline";
      syncBadgeEl.textContent = "SYNC ATTENTION NEEDED";
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
}

function loadCluster() {
  var s = document.createElement("script");
  s.src = "/ext/unifi-bridge/cluster.js?_=" + Date.now();
  s.onload = function() {
    if (window.unifiBridgeCluster) {
      updateClusterUI(window.unifiBridgeCluster);
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
    if (!autoRefreshTimer) autoRefreshTimer = setInterval(loadStatus, 5000);
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
    document.getElementById("input_controller_url").value = "http://" + detectedGatewayIP + ":8080/inform";
  } else {
    customDiv.style.display = "block";
  }
}

function syncClusterNow() {
  document.form.action_script.value = "start_unifi_bridge_sync";
  document.form.action_mode.value = "apply";
  document.form.action_wait.value = "4";

  if (typeof showLoading === "function") {
    showLoading(4);
  }
  document.form.submit();

  setTimeout(function() {
    loadCluster();
    loadStatus();
  }, 4500);
}

function applySettings() {
  var isEnabled = document.getElementById("unifibridge_enabled_1").checked ? "1" : "0";
  var isAutoGw = document.getElementById("unifibridge_auto_1").checked ? "1" : "0";
  var ctrlUrl = document.getElementById("input_controller_url").value.trim();
  var model = document.getElementById("select_model").value;
  var interval = document.getElementById("select_interval").value;

  if (isAutoGw === "1") {
    ctrlUrl = "http://" + detectedGatewayIP + ":8080/inform";
  }

  document.form.unifibridge_enabled.value = isEnabled;
  document.form.unifibridge_auto_gateway.value = isAutoGw;
  document.form.unifibridge_controller_url.value = ctrlUrl;
  document.form.unifibridge_model.value = model;
  document.form.unifibridge_interval.value = interval;

  document.form.action_script.value = "start_unifi_bridge_apply";
  document.form.action_mode.value = "apply";
  document.form.action_wait.value = "4";

  if (typeof showLoading === "function") {
    showLoading(4);
  }
  document.form.submit();

  setTimeout(function() {
    loadStatus();
    loadCluster();
  }, 4500);
}

function initial(){
  SetCurrentPage();
  if (typeof show_menu === "function") {
    show_menu();
  }
  document.getElementById("detected_gw_text").textContent = detectedGatewayIP;
  loadStatus();
  autoRefreshTimer = setInterval(loadStatus, 5000);
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
<input type="hidden" name="unifibridge_enabled" value="1">
<input type="hidden" name="unifibridge_auto_gateway" value="1">
<input type="hidden" name="unifibridge_controller_url" value="http://10.100.1.1:8080/inform">
<input type="hidden" name="unifibridge_model" value="U6M">
<input type="hidden" name="unifibridge_interval" value="10">

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
                    <div class="formfonttitle" id="scripttitle" style="text-align:center;">UniFi AP Bridge</div>
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
                      </div>
                      <div class="stat_card">
                        <div class="stat_label">5 GHz (High-Speed)</div>
                        <div class="stat_num" style="color:#3498db;" id="stat_5g">-</div>
                      </div>
                    </div>

                    <!-- Cluster Nodes & Synchronization Table -->
                    <table width="100%" border="1" align="center" cellpadding="2" cellspacing="0" bordercolor="#6b8fa3" class="FormTable SettingsTable" style="border:0px; margin-bottom:12px;" id="table_cluster">
                      <thead class="collapsible-jquery">
                        <tr>
                          <td colspan="6">
                            <div style="display:flex; justify-content:space-between; align-items:center;">
                              <span>Cluster Access Points & Synchronization Status</span>
                              <span id="cluster_sync_badge" class="badge_online" style="font-size:10px; margin-right:5px;">ALL NODES IN SYNC</span>
                            </div>
                          </td>
                        </tr>
                      </thead>
                      <thead>
                        <tr bgcolor="#2F3A3E" style="color:#bdc3c7; font-size:11px; text-align:left;">
                          <th style="padding:6px 10px; width:22%;">Access Point</th>
                          <th style="padding:6px 10px; width:15%;">Role</th>
                          <th style="padding:6px 10px; width:15%;">IP Address</th>
                          <th style="padding:6px 10px; width:14%;">Status</th>
                          <th style="padding:6px 10px; width:17%;">Clients</th>
                          <th style="padding:6px 10px; width:17%;">Sync Status</th>
                        </tr>
                      </thead>
                      <tbody id="cluster_nodes_tbody">
                        <tr>
                          <td colspan="6" style="padding:10px; text-align:center; color:#bdc3c7; background-color:#475a5f;">Loading cluster node status...</td>
                        </tr>
                      </tbody>
                    </table>
                    <div style="display:flex; justify-content:space-between; align-items:center; margin:-6px 0 14px 2px; font-size:11px; color:#bdc3c7;">
                      <span>Last Cluster Sync: <span id="last_sync_time" style="color:#ecf0f1; font-weight:bold;">-</span></span>
                      <input class="button_gen" type="button" value="Sync Cluster Now" onclick="syncClusterNow();" style="padding:2px 10px; font-size:11px;" />
                    </div>

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
                        <td class="settingvalue"><code id="controller_val">http://10.100.1.1:8080/inform</code></td>
                      </tr>
                      <tr>
                        <td class="settingname">Last Heartbeat Exchange</td>
                        <td class="settingvalue" id="heartbeat_val">-</td>
                      </tr>
                    </table>

                    <div style="line-height:10px;">&nbsp;</div>

                    <!-- Configuration Table -->
                    <table width="100%" border="1" align="center" cellpadding="2" cellspacing="0" bordercolor="#6b8fa3" class="FormTable SettingsTable" style="border:0px;" id="table_config">
                      <thead class="collapsible-jquery">
                        <tr>
                          <td colspan="2">
                            <span>Configuration</span>
                            <span style="font-size:11px; color:#bdc3c7; font-weight:normal; margin-left:10px;">(Settings automatically propagate to all cluster nodes)</span>
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
                            <label><input type="radio" name="radio_auto" id="unifibridge_auto_1" value="1" checked onchange="toggleControllerInput();"> Auto-detect Gateway (<code>http://<span id="detected_gw_text">10.100.1.1</span>:8080/inform</code>)</label>
                          </div>
                          <div style="margin-top:6px;">
                            <label><input type="radio" name="radio_auto" id="unifibridge_auto_0" value="0" onchange="toggleControllerInput();"> Custom Inform URL</label>
                          </div>
                          <div id="custom_url_div" style="display:none;margin-top:6px;padding-left:22px;">
                            <input type="text" class="input_32_table" id="input_controller_url" value="http://10.100.1.1:8080/inform" maxlength="128" placeholder="http://ip-or-host:8080/inform">
                          </div>
                        </td>
                      </tr>
                      <tr>
                        <td class="settingname">
                          Emulated AP Hardware Profile<br/>
                          <span style="font-size:10px;color:#bdc3c7;">UniFi hardware model reported to controller</span>
                        </td>
                        <td class="settingvalue">
                          <select class="input_option" id="select_model">
                            <option value="U6M" selected>UniFi 6 Mesh (U6M) - Dual/Tri-Band Wi-Fi 6 (Recommended)</option>
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
                      <input class="button_gen" type="button" value="Apply Settings" onclick="applySettings();" />
                    </div>

                    <!-- Live Heartbeat Events -->
                    <div style="margin-top:18px;">
                      <div style="display:flex; justify-content:space-between; align-items:center;">
                        <span style="font-size:12px; font-weight:bold; color:#ecf0f1;">Live Heartbeat Events</span>
                        <div>
                          <label style="font-size:11px; color:#bdc3c7; cursor:pointer;">
                            <input type="checkbox" id="auto_refresh_chk" checked onclick="toggleAutoRefresh(this);"> Auto-refresh (5s)
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
