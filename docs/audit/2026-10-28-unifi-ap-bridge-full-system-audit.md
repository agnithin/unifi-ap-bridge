# UniFi AP Bridge — Comprehensive Codebase Audit & System Review

**Audit Date:** 2026-10-28  
**Scope:** Full repository review of [`unifi-ap-bridge`](/Users/nithinag/workspace/unifi-ap-bridge) (Branch: `dev`, Commit: `c520804`, Version: `1.2.6`)  
**Target Hardware:** ASUS ZenWiFi XT8 (Broadcom BCM4908 quad-core ARMv7, 512 MB RAM, Linux 4.1.52 ASUSWRT-Merlin)  
**Cluster Status:** Main AP (`10.100.1.10`, PID 27247), Node AP (`10.100.1.11`, PID 24033) — Active (GCM), 60 connected stations.

---

## 1. Executive Summary

A comprehensive architectural and code audit of the `unifi-ap-bridge` project was performed across all subsystems: the daemon runtime (`cmd/unifi-bridge`), inform cryptography and state management (`pkg/inform`), Broadcom telemetry collection (`providers/broadcom`), cluster orchestration and atomic update scripts (`scripts`), and the embedded management UI (`webui`).

The overall system architecture is exceptionally disciplined for an embedded router daemon:
- **Low Footprint:** Resident memory (RSS) is only ~10.9 MB per node on an unswapped 512 MB router, with 0% CPU utilization outside 10-second sampling bursts.
- **Strict Hardening:** All private keys, session tokens, and configuration state are constrained to `0600` permissions with atomic temporary file swapping (`.tmp` → rename).
- **Inform Protocol Fidelity:** AES-GCM (16-byte nonce, 40-byte AAD header) and legacy AES-CBC inform framing match native Ubiquiti AP behavior.
- **Multi-Bridge Isolation:** Recent v1.2.6 fixes verified live on UniFi Network 10.6.109: Port 4 correctly reports learned MACs on VLAN 20, and cross-bridge ARP poisoning has been eliminated.

This audit identifies **4 actionable code fixes**, **3 native UniFi capability improvements**, and **operational recommendations** for release management and controller alignment.

---

## 2. Core Operational Findings & Fixes

### Finding 1: Transient Client Departure Race in `getStationsForIfaceContext`
* **Severity:** Medium (Availability / Robustness)
* **Location:** `providers/broadcom/asus.go#L721-L727`
* **Symptom:** Transient `telemetry sample failed: collect vaps: read eth4 station <mac> details: exit status 211` errors causing full heartbeat failure.
* **Analysis:**
  `getStationsForIfaceContext` first fetches associated MACs using `wl -i <iface> assoclist`. It then loops through each station and invokes `wl -i <iface> sta_info <rawMAC>`. When a client roams between APs or disconnects during the brief milliseconds between `assoclist` and `sta_info`, the Broadcom driver returns exit code `211` (`wl: Bad Address`).
  Because `getStationsForIfaceContext` treats any `err != nil` from `sta_info` as fatal, the error aborts `GetVAPsContext`, which in turn aborts the entire `Collect` sample.
* **Recommended Fix:**
  Detect when `sta_info` fails with non-zero exit status or empty response on an unassociated client. Log a debug-level event and skip that departed station instead of failing the entire VAP poll:
  ```go
  infoRaw, err := p.wlCmdContext(ctx, iface, "sta_info", rawMAC)
  if err != nil || len(infoRaw) == 0 {
      // Station roamed or disassociated between assoclist and sta_info read.
      continue
  }
  ```

---

### Finding 2: Packet Capture Interface Restriction to `br0`
* **Severity:** Low (Feature Limitation / Multi-Bridge Gaps)
* **Location:** `pkg/capture/capture.go#L52-L54`
* **Symptom:** Attempting to capture packets on a client assigned to an isolated VLAN (e.g. `br20` on Port 4) fails with `packet capture is restricted to interface br0`.
* **Analysis:**
  The validation logic in `validate(req Request)` strictly requires `req.Interface == "br0"`. On routers configured with multiple bridges for VLAN isolation (such as `br0` for LAN and `br20` for IoT), clients on non-management VLANs cannot be debugged with the packet capture utility.
* **Recommended Fix:**
  Allow capture on any active Linux bridge by validating `strings.HasPrefix(req.Interface, "br")` and checking that the bridge exists in the system directory `/sys/class/net/<iface>/bridge`.

---

### Finding 3: Default Version Constant Drift in `cmd/unifi-bridge/main.go`
* **Severity:** Low (Maintainability / Developer Experience)
* **Location:** `cmd/unifi-bridge/main.go#L26`
* **Analysis:**
  `version.txt` was bumped to `1.2.6`, and release builds inject `-X main.version=1.2.6`. However, the fallback constant in `main.go` remains `"1.2.5"`. Any local `go run ./cmd/unifi-bridge` or un-injected developer build will report the stale version string.
* **Recommended Fix:**
  Update the fallback constant to match `version.txt` (`version = "1.2.6"`).

---

### Finding 4: GitHub Actions Release Workflow Triggers
* **Severity:** Medium (DevOps / Release Pipeline)
* **Location:** `.github/workflows/build-and-release.yml`
* **Analysis:**
  The workflow trigger is currently configured as:
  ```yaml
  on:
    push:
      tags:
        - 'v*'
      branches:
        - dev
  ```
  If `dev` is merged to `main` without explicitly tagging a release (e.g., `git tag v1.2.6`), GitHub Actions will **not** trigger. Routers tracking `main` in `update.sh check main` query the `/releases/latest` endpoint, which would never be updated.
* **Recommended Fix:**
  Add a release SOP or workflow rule: When promoting `dev` to `main`, always create and push a semantic version tag `vX.Y.Z` matching `version.txt` to trigger the production build pipeline.

---

## 3. Native UniFi Capability Alignment & Feature Gaps

| Capability | Current State in `unifi-ap-bridge` | Native UniFi AP Behavior | Recommended Action |
| :--- | :--- | :--- | :--- |
| **Locate Device** | Ignored by `command_dispatch.go`. Observed envelope recorded. | Flashes LED ring / status LED for 10–15 seconds. | Implement `/sbin/led_ctrl` or `nvram set AllLED` blink routine in `controller_commands.go`. |
| **Radio Name Consistency** | `CompactRadioNames` enabled (`wifi-ng`, `wifi-na`, `wifi-na1`). | U6-IW uses `wifi0`, `wifi1`. | Controlled forget/re-adopt of Main AP to purge stale legacy `wifi-ng-1` record. |
| **AirView History (Upper 5GHz)** | `wifi-na1` reports active statistics, but UniFi historical overview aggregates 5G. | Displays per-radio historical AirView graphs. | No change needed; Asus WebUI passive RF trend provides measured raw values. |
| **Non-WiFi Interference (`cu_interf`)** | Broadcom CCA interference exposed in WebUI; omitted from inform. | UniFi expects proprietary duty-cycle metric. | Maintain current conservative posture: avoid fabricating synthetic `cu_interf`. |
| **Channel Width Display** | Emits `HT: "80"` on `wifi-na1`. Controller overview shows `40MHz`. | Fixed by controller model profile. | Documented controller UI bug; driver runs at verified 80 MHz channel 161. |
| **Switch Port Management** | Emulates U6-IW with `fw_caps: 16`. Ports 1–4 visible with live FDB and VLANs. | Native switch port profiles & PoE control. | Ports 1–4 fully functional; PoE correctly flagged as unsupported. |

---

## 4. Hardware Constraints & Resource Optimization

### Memory Footprint on Embedded Linux 4.1.52 (ASUSWRT)
```
Memory Status (ZenWiFi XT8 Node):
Total: 510,016 kB (~500 MB)
Used:  420,740 kB
Free:   89,276 kB (Buffers: 5,424 kB, Cached: 143,348 kB)
Swap:        0 kB (No swap on flash storage)
```
- **Virtual Memory Allocator:** Go runtime reserves 560 MB of virtual address space (`VmSize`), which is normal for Go 64-bit/32-bit arena pre-allocations.
- **Resident Set Size (Actual RAM):** `VmRSS` is verified at **`10,964 kB`** (~10.9 MB).
- **Execution from RAM:** `start.sh` copies the binary from JFFS flash to `/tmp/unifi-bridge/unifi-bridge` before execution, eliminating read wear on flash storage during execution.
- **Log Bounding:** `boundedLogWriter` limits stderr logs to 1 MB, and status files in `/tmp/var/wwwext/` are atomic and bounded.

---

## 5. Security & Isolation Audit

1. **Remote Execution Guard:**
   `command_dispatch.go` rejects arbitrary strings from the controller. The only supported command is `reboot`, which requires:
   - Verified AES-GCM response encryption.
   - Active adopted state under a non-default secret key.
   - Explicit `ControllerRebootEnabled: true` in `config.json`.
   - Fixed `/sbin/reboot` execution with a 60-second cooldown and deduplicated `_id` tracking.
2. **File Permissions:**
   - Secrets (`state.json`, `.ssh/cluster_key`, `config.json`) are enforced `0600`.
   - Update staging directories use `umask 077`.
   - Temporary bundles verify SHA-256 digests and reject non-regular file types (e.g. symlinks, hardlinks, SUID/SGID bits).
3. **No External Dependencies:**
   The Go module contains zero third-party dependencies (`go.mod` is standard library only), minimizing supply chain attack surface.

---

## 6. Prioritized Action Plan

1. **P1 — Implement Station Departure Exception Handling:**
   Modify `getStationsForIfaceContext` to skip stations whose `sta_info` query returns exit 211 / `Bad Address`, preventing transient client drops from aborting the entire AP poll.
2. **P2 — Permit Packet Capture on VLAN Bridges:**
   Update `validate` to accept any valid bridge interface `br*` present in sysfs.
3. **P3 — Version Constant Alignment:**
   Sync `version = "1.2.6"` in `cmd/unifi-bridge/main.go`.
4. **P4 — Document Promotion Tagging Procedure:**
   Ensure `git tag v1.2.6` is pushed when promoting `dev` to `main` to trigger the production GitHub Actions builder.
5. **P5 — Locate LED Command (Optional Improvement):**
   Add a safe 10-second LED blink routine in `controller_commands.go` responding to UniFi's "Locate" button.
