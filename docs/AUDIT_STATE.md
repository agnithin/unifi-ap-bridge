# Audit State & Current Operations

**Last live verification:** 2026-10-24, approximately 22:23 PDT. This document records dated evidence; it does not imply a new live check on each documentation edit.

**Status:** Both APs and controller heartbeats healthy. Development update deployed. Gateway parent-port topology, credential rotation, and additional failure-path review remain open.

**Release track:** `dev`, daemon `1.2.2`, deployed code commit `1b6ce07affb18f30ef86a822e5939b9217b3648f`. Documentation commits may be newer without changing the deployed code. Do not merge to `main` without explicit operator approval.

**Repository:** `/Users/nithinag/workspace/unifi-ap-bridge`.

See [README](../README.md) for usage and [ARCHITECTURE.md](ARCHITECTURE.md) for implementation details.

## 1. Latest Verified Deployment — 2026-10-24, approximately 22:20–22:23 PDT

**Operator authorization:** Commit and push changes to the dev build and update the addon to the latest version. No merge to `main` was performed.

### Published source and build

- `5a40d05`: daemon, cluster, permission, key validation, logging, CI, and audit fixes.
- The first hosted run caught a real portability defect: dash discarded transfer stdin for a background SSH command despite `<&0`. The APs were not updated from that failed build.
- `1b6ce07`: preserve caller stdin on file descriptor 3 before launching background SSH. Local tests and the Ubuntu CI regressions passed afterward.
- Successful build/release: https://github.com/agnithin/unifi-ap-bridge/actions/runs/40165119811
- The published ARMv7 archive matched the release SHA-256 manifest. Running its version command before installation reported `1.2.2`, full commit `1b6ce07affb18f30ef86a822e5939b9217b3648f`, build date `2026-10-25` (UTC).

### Installation and verification

Main's existing updater ran `update.sh apply dev`, restarted the telemetry bridge, and propagated the package/scripts/configuration to Node through the updated cluster script. Rollback archives were created first on each AP at `/tmp/unifi-bridge-before-dev-1b6ce07/addon.tar.gz` inside private directories. These are RAM-backed temporary backups and will not survive reboot; they contain sensitive addon state and must remain private.

| Check | Main (`10.100.1.10`) | Node (`10.100.1.11`) |
|---|---|---|
| Runtime version | `1.2.2` | `1.2.2` |
| Runtime commit | `1b6ce07affb18f30ef86a822e5939b9217b3648f` | Same |
| Release track | `dev` | `dev` |
| New daemon PID | `8418` | `15893` |
| Adoption / cipher | Adopted, GCM | Adopted, GCM |
| Recent logs | Repeated successful `noop` heartbeats | Repeated successful `noop` heartbeats |
| Station count | 28 | 32 |
| Addon / SSH directory modes | `0755` / `0700` | `0755` / `0700` |
| Persistent state file mode | `0600` | `0600` |
| Installed script, WebUI, version checksums | Match committed source | Match committed source |
| Controller state / model | `ONLINE` / `U6 IW` | `ONLINE` / `U6 IW` |
| Controller heartbeat age at final query | 5.2 seconds | 2.2 seconds |
| Controller Port 1 | UP, 2500 Mbps | UP, 2500 Mbps |
| Controller Port 4 | DOWN | UP, 1000 Mbps |

All seven checked installed files matched local committed source via SHA-256: apply handler, cluster script, CLI, updater, watchdog, WebUI ASP, and version file. The state files retained their pre-update sampled modification times; no re-adoption was needed. A total of 60 associated stations was observed before and after; this is not a packet-loss or zero-disruption measurement.

The cluster table briefly showed cached pre-restart status during the update. After both new daemons produced fresh telemetry, Main's `sync_nodes.sh status` regenerated the view: both nodes `RUNNING` at `1.2.2`, with Node labeled `Version Match (Config Unverified)`. No production Wi-Fi service restart was requested; only the telemetry addon was restarted.

### Remaining open work

- **A1/A3:** Permission enforcement and deployed version/provenance now verified on both APs.
- **A2:** Official API access and current-document credential removal verified. Rotation of the previously exposed, still-active credential remains open.
- **A4/A5:** Implemented regressions and normal live operation passed. Controlled outage/recovery tests and the wider architecture review remain open; do not claim them complete.
- **A6:** Daemon log cap is deployed. Watchdog log growth and broader persistent-write review remain open.
- **Topology:** The final official device responses still return `uplink: null` for both APs. Gateway parent-port topology remains unresolved despite correct physical/controller port links.
- **Release:** Keep `dev`. No approval to merge into `main` has been given. This deployment record is documentation-only and does not change the published build's code identity.


## 2. Current Action Register

| ID | Status | Current disposition and next step |
|---|---|---|
| A1 — Key directory security | Deployed and verified | Both addon directories `0755`, SSH directories `0700`, state files `0600`. Installer/updater/cluster paths enforce the directory modes. Preserve observed private-key mode `0600`. |
| A2 — Controller access / credential hygiene | Partly resolved | Official Integration API works; literal credential removed from current docs. The key remains active in earlier Git history. Coordinate rotation and update its consumers; do not treat documentation removal as revocation. |
| A3 — Version / provenance | Deployed and verified | Both APs identify as `1.2.2`, full commit `1b6ce07…`, UTC build date `2026-10-25`. Installed source files match committed checksums. The prior `1.2.1` default was stale, but the provenance of the older binaries was not independently proven. |
| A4 — Daemon and architecture review | Fixes deployed; review open | Interval normalization, refreshed uptime, truthful heartbeat timestamps, adoption recovery threshold, malformed-key rejection, and bounded daemon logs are implemented and tested. Controlled outage/recovery checks, wider provider/schema/WebUI edge-case review, and sustained resource observations remain open. |
| A5 — Cluster trust / health | Fixes deployed; follow-up open | SSH watchdog, preserved transfer input, PID/freshness checks, and honest version-match labels passed tests and normal live operation. Preserve single `-y` trust-on-first-use behavior for now; dedicated-key migration and controlled unreachable-node tests remain separate follow-up decisions. |
| A6 — Flash and RAM writes | Partly resolved | Runtime telemetry/logs are under `/tmp`; daemon log cap is 1 MiB. State writes use atomic rename on adoption/configuration changes. No flash-wear benchmark is required; verify watchdog log growth and remaining persistent write paths. Avoid absolute zero-wear claims. |
| A7 — Feature backlog | Deferred; not a v1.2.2 blocker | Dynamic WLAN mapping, guest tagging, and roam events remain future features. Current association telemetry does not claim roam-event generation. |
| A8 — Merge gate | Open | Stay on `dev`. Resolve or explicitly disposition current topology/security/reliability follow-up, retain passing checks and build provenance, and obtain explicit approval before merging to `main`. |

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
