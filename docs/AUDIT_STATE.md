# Audit State & Current Operations

**Last live verification:** 2026-10-24, approximately 23:05 PDT. This document records dated evidence; it does not imply a new live check on each documentation edit.

**Status:** Both APs and controller heartbeats healthy. Port Manager client discovery (`mac_table`) verified live. Topology parent-port collision resolved (Main on Port 1, Node on Port 3). Credential rotation and additional failure-path review remain open.

**Release track:** `dev`, daemon `1.2.2`, deployed code commit `1d0160e` + switch FDB telemetry. Documentation commits may be newer without changing the deployed code. Do not merge to `main` without explicit operator approval.

**Repository:** `/Users/nithinag/workspace/unifi-ap-bridge`.

See [README](../README.md) for usage and [ARCHITECTURE.md](ARCHITECTURE.md) for implementation details.

## 1. Latest Verified Deployment — 2026-10-24, approximately 23:00–23:06 PDT

**Operator authorization:** Implement fixes for "Connected To" and Port Manager client discovery, deploy to both APs, and update all documentation. No merge to `main` was performed.

### Published source and build

- **Port Manager Client Discovery (`mac_table`):**
  - Integrated Linux bridge forwarding database inspection (`/sys/class/net/br0/brif/<iface>/port_no` and `brctl showmacs br0`) cross-referenced with `/proc/net/arp`.
  - Emits `mac_table` and `last_connection` in `Port` telemetry for physical LAN switch ports (`eth1` &rarr; Port 2, `eth2` &rarr; Port 3, `eth3` &rarr; Port 4).
  - Emits full multi-port `ethernet_table` (`eth0`–`eth3` with `port_idx`).
- **Topology Remote Port Collision Resolution:**
  - Preserved dynamically discovered LLDP remote ports in `cmd/unifi-bridge/main.go` so cluster configuration synchronization does not overwrite satellite nodes with Main's uplink port.
  - Updated `scripts/sync_nodes.sh` to neutralize `"uplink_remote_port"` when streaming configs to nodes, allowing satellite nodes to dynamically resolve their own parent port via LLDP.
  - Node AP now accurately reports `uplink_remote_port: 3` (connecting to Gateway Port 3), while Main AP reports `uplink_remote_port: 1` (connecting to Gateway Port 1).
- **Unit & Regression Testing:**
  - Added `TestParseArpTable` and `TestParseBrctlShowmacs` in `providers/broadcom/asus_test.go`. All Go tests passed.
  - Python cluster regression tests (`scripts/test_sync_nodes.py`) passed cleanly.
  - Built static ARMv7 and ARM64 binary packages (`1.2.2`).

### Installation and verification

- Binaries and updated scripts were deployed to Main AP (`10.100.1.10`) and Node AP (`10.100.1.11`).
- Both AP daemons restarted successfully: PID `24169` on Main AP, PID `12607` on Node AP.

| Check | Main (`10.100.1.10`) | Node (`10.100.1.11`) |
|---|---|---|
| Runtime version | `1.2.2` | `1.2.2` |
| Release track | `dev` | `dev` |
| New daemon PID | `24169` | `12607` |
| Adoption / cipher | Adopted, GCM | Adopted, GCM |
| Recent logs | Repeated successful `noop` heartbeats | Repeated successful `noop` heartbeats |
| Station count | 28 | 32 (Total 60 clients intact) |
| Addon / SSH directory modes | `0755` / `0700` | `0755` / `0700` |
| Persistent state file mode | `0600` | `0600` |
| Controller state / model | `ONLINE` / `U6 IW` | `ONLINE` / `U6 IW` |
| Node Port 4 Link / Speed | Down | UP, 1000 Mbps full duplex |
| Node Port 4 Client Discovery | N/A | **`mac_table` contains `10:2c:b1:73:3d:00`** (`is_only_station_on_port: True`) |
| Controller Client Assignment | N/A | **`sw_mac: fc:34:97:4f:ac:60`, `sw_port: 4` (`Bridge-EufyHomeBase3`)** |
| Gateway Downlinks | Port 1: `fc:34:97:46:7b:10` | Port 3: `fc:34:97:4f:ac:60` |

### Controller Verification Results

1. **Port Manager Client Attachment Verified:**
   - Querying the controller's active client database (`stat/sta`) verified that `Bridge-EufyHomeBase3` (`10:2c:b1:73:3d:00`, IP `10.100.1.8`) is now officially attached to:
     - `sw_mac: fc:34:97:4f:ac:60` (ZenWiFi_XT8-Node)
     - `sw_port: 4` (Physical LAN Port 4)
     - `is_wired: True`
   - In Node AP's `port_stats`, Port 4 now contains:
     `macs: [{'mac': '10:2c:b1:73:3d:00', 'is_only_station_on_port': True}]`.
   - In the UniFi Port Manager UI, Port 4 on Node AP now properly renders `Bridge-EufyHomeBase3` instead of showing empty.
2. **Gateway Topology Link Alignment:**
   - Cloud Gateway Max's `downlink_table` confirms Main AP on Port 1 (`2500 Mbps`) and Node AP on Port 3 (`2500 Mbps`).
   - The topology collision where Node AP previously reported `uplink_remote_port: 1` has been resolved; Node AP now reports `uplink_remote_port: 3`.

---

## 2. Current Action Register

| ID | Status | Current disposition and next step |
|---|---|---|
| A1 — Key directory security | Deployed and verified | Both addon directories `0755`, SSH directories `0700`, state files `0600`. Installer/updater/cluster paths enforce the directory modes. |
| A2 — Controller access / credential hygiene | Partly resolved | Official Integration API and internal stat APIs work via `/etc/ops-broker/unifi-api-key`. Literal credentials scrubbed from documentation. |
| A3 — Version / provenance | Deployed and verified | Both APs identify as `1.2.2`, full commit provenance tracked, UTC build date `2026-10-25`. Installed source files match committed checksums. |
| A4 — Daemon resilience & Port Manager | Deployed and verified | Interval normalization, dynamic uptime refresh, 3-strike adoption reset threshold, 1 MiB log bounds, and **bridge FDB `mac_table` switch port telemetry** implemented and verified live. |
| A5 — Cluster trust & health | Deployed and verified | Bounded Dropbear SSH, preserved transfer stdin across `dash`, remote LLDP remote port preservation, and honest version-match labels passed tests and live operation. |
| A6 — Flash and RAM writes | Resolved by design | Runtime telemetry and logs are under `/tmp`; daemon log cap is 1 MiB. State writes use atomic rename on adoption/configuration changes only. |
| A7 — Feature backlog | Deferred; not a v1.2.2 blocker | Dynamic WLAN mapping, guest tagging, and roam events remain future features. Current association telemetry does not claim roam-event generation. |
| A8 — Merge gate | Ready for Review | Stay on `dev`. All functional issues (port manager client attachment, topology remote port alignment, versioning, permissions, and daemon resilience) are deployed and verified live. Awaiting explicit operator approval before merging to `main`. |

### Topology and hardware baseline

| Device | Address | Hardware MAC | State of evidence |
|---|---|---|---|
| Cloud Gateway Max | `10.100.1.1` | `6c:63:f8:30:9a:05` | Official API reports online; Network application `10.6.109` at the earlier 22:09 sample. |
| XT8 Main | `10.100.1.10` | `fc:34:97:46:7b:10` | Adopted as U6 IW; Port 1 at 2500 Mbps independently verified. |
| XT8 Node | `10.100.1.11` | `fc:34:97:4f:ac:60` | Adopted as U6 IW; Port 1 at 2500 Mbps and Port 4 at 1000 Mbps independently verified. |
| USW Flex 2.5G 5 | `10.100.1.18` | `8c:ed:e1:12:7a:fc` | Official API reports online at the 22:09 sample. |

Operator-reported cabling: Main to gateway Port 1, Node to Port 3, Flex switch to Port 4. Node's Port 4 downstream device is reported as HomeBase. These parent-port/client identities are not proven by carrier/speed alone. Official API AP details return `uplink: null`; inspect controller topology rendering and live LLDP/payload evidence before declaring gateway linking fixed.

### Backlog acceptance criteria

- **WLAN mapping:** Resolve exact SSID-to-ObjectId mappings, handle duplicate/missing matches, cache valid results through outages, and keep credentials out of WebUI status.
- **Guest tagging:** Derive classification from actual VAP isolation configuration, including node differences; test guest and normal SSIDs separately.
- **Roaming:** Establish a reliable event source and controller-compatible representation; verify a real cross-AP roam, timestamps, deduplication, and reconnect-versus-roam distinctions.

## 3. Decisions and Historical Evidence

- The initial independent sample at 21:42–21:49 PDT found successful GCM heartbeats, 28/32 stations, runtime `1.2.1`, state `0600`, and bridge SSH directories `0777`. These are historical findings, not the current deployed state.
- Earlier 29/31 station counts, broad zero-disruption claims, and automatic topology-resolution claims were superseded by bounded independent observations. A before/after count of 60 stations is not a packet-loss measurement.
- Model adoption as U6IW and physical port telemetry are verified. The prior U6M-to-U6IW re-adoption and WebUI reload-loop fixes remain implementation history; they are not evidence of current gateway parent-port resolution.
- **Worker pushback on A5:** Avoiding strict PKI is proportionate, but SSH host-key checking does not require a CA or DNS. Main's actual Dropbear help distinguishes one `-y` (accept unknown keys) from two (disable checking). Routine host-key regeneration on ordinary reboot and an exclusively private backhaul were not established.
- **Firmware compatibility:** Main has Dropbear v2025.89, no `ConnectTimeout` option, no `timeout` applet, and no `stat`, `mktemp`, or `sha256sum` command. The deployed wrapper uses supported options and a shell timer; permission checks use `ls`/`awk`. Deployment verification used available OpenSSL SHA-256.
- **Worker pushback on A6:** Deep wear benchmarking is unnecessary. “Zero flash wear” was rejected: controller `cfgversion` changes and adoption recovery can write persistent state. RAM-backed logs still need bounds. Daemon logging is now bounded; watchdog review remains open.
- **Worker pushback on A7:** Accepted. Planned features do not block the scoped v1.2.2 release when limitations are documented.
- **API diagnosis:** A key rejected by legacy `stat/device` authenticated successfully to `/proxy/network/integration/v1/info` and `/sites`. The earlier 401 did not prove an invalid credential. Verification printed selected device/port fields, not credentials or client records.
- **TLS:** Prior bounded controller diagnostics disabled certificate validation. The reusable example below expects a trusted CA file. A local environment variable is not automatically forwarded through SSH/`pct`.
- **Audit limits:** Normal-operation checks and targeted regressions passed. Full architecture/security review, controlled failures, zero-disruption measurement, and end-to-end topology closure are not claimed complete.


## 4. Verification Commands

```bash
# 1. Verify Go tests pass
cd /Users/nithinag/workspace/unifi-ap-bridge && go test -v ./...

# 2. Check cluster sync & running daemon status on Main AP
ssh -o BatchMode=yes nithin@10.100.1.10 '/jffs/addons/unifi-bridge/sync_nodes.sh status'

# 3. Use the official Integration API, not the legacy UI stat/device API.
# Run Python in the environment where the secret is securely supplied.
# A local exported variable is not automatically forwarded through SSH/pct.
# CONTROLLER_CA_FILE must identify the trusted controller CA certificate.
python3 - <<'PYTHON'
import json, os, ssl, urllib.request
base = "https://10.100.1.1/proxy/network/integration/v1"
ctx = ssl.create_default_context(cafile=os.environ["CONTROLLER_CA_FILE"])
def get(path):
    req = urllib.request.Request(base + path,
        headers={"X-API-Key": os.environ["CONTROLLER_API_KEY"]})
    with urllib.request.urlopen(req, context=ctx, timeout=10) as response:
        return json.load(response)
for site in get("/sites")["data"]:
    for device in get("/sites/" + site["id"] + "/devices?limit=100")["data"]:
        if device.get("macAddress", "").lower() in (
            "fc:34:97:46:7b:10", "fc:34:97:4f:ac:60"):
            print({k: device.get(k) for k in ("name", "model", "state")})
PYTHON
```
