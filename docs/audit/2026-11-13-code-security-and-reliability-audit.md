# Code security and reliability audit — November 13, 2026

## Scope and result

Audited source at `81aa49b` on `dev`, using isolated branch `audit/unifi-bridge-20261113`. Reviewed daemon exchanges, adoption persistence, controller command execution, wire parsing, HTTP transport, telemetry/syslog collection, packet capture boundaries, shell lifecycle/update/sync flows, WebUI rendering, and release verification. Existing November 4 findings were checked against current implementation rather than assumed unresolved.

This is a repository audit with local regression and cross-build verification. No routers, controller sessions, credentials, installed services, or network configuration were accessed or changed. It does not certify hardware behavior or prove absence of additional defects.

## Findings fixed

| ID | Priority | Original behavior and consequence | Implemented correction |
| --- | --- | --- | --- |
| A1 | High | `executePlatformMacFilter` silently invented disabled/empty snapshots when reads failed, ignored clear failures, checked only mode, omitted the failing interface from rollback, and reused an expired context. Partial driver writes could leave unintended filtering active or remove policy. | Require valid snapshots on every interface before writing; refuse ASUS allow-list replacement; check each clear/write; verify mode and exact MAC list; restore all touched interfaces including the failing one in reverse order with a fresh bounded context. Preserve inactive stored lists. Return combined restoration errors and log failures. Deauthenticate only after all policy readbacks succeed. |
| A2 | High | `StateManager.Load` applied inconsistent validation to primary/backup candidates, accepted corrupt backup keys on one path, and skipped backup recovery for valid JSON with invalid schema/key. `Save` used predictable `.tmp`, overwrote backup with new data instead of retaining a prior generation, and did not reliably enforce existing file permissions or report directory sync errors. | Share complete identity/key validation across candidates, reject adopted state without a key, try backup after any invalid primary, refuse bootstrap over an invalid backup. Use private unique temporary files and atomic renames for primary and validated previous generation; preserve valid backup when primary is corrupt; propagate write/sync errors. |
| A3 | High | Command dispatch required GCM, but subsequent adoption-state processing accepted CBC replies even for adopted sessions. Reset/key/config changes therefore lacked the same authenticated-response requirement. CBC encryption does not provide GCM integrity. | Gate the whole adopted response before capture, dispatch, or state processing. Reject non-GCM replies; preserve bootstrap CBC compatibility. Regression test uses actual encrypted reset packets. |
| A4 | Medium | Uncompressed decrypted JSON bypassed the 64 KiB limit; decoder accepted `null` or a valid object followed by additional content. | Enforce the JSON byte limit in both compressed and uncompressed paths, require an object, and require EOF after its value. Keep known-answer CBC/GCM fixtures passing. |
| A5 | Medium | Default HTTP redirect handling could forward an Inform POST to another endpoint on 307/308 or silently switch to GET on other redirects. | Disable automatic redirects and return their HTTP status as an error. Test 301/302/303/307/308 without making external requests. |
| A6 | Medium | Syslog follower detected rotation only by size and discarded partial lines at EOF. A larger replacement file could strand the reader on the old inode; split writes lost events. | Compare file identity as well as size, retain partial lines across polls, and bound line assembly to 64 KiB. Test larger replacement files and split writes. |
| A7 | Medium | Payload advertised `fixed_ap_available: true` although controller-driven AP locking has no enforcement path. | Advertise false, consistent with existing architecture and the November 4 audit. Existing saved controller client-lock preferences are not enforcement guarantees. |
| A8 | Low | Default development binary version and README badge said 1.2.12 while `version.txt` said 1.2.15. README/log text claimed transport backoff absent from the exchange loop and guaranteed atomic MAC rollback across independent driver commands. | Align default version/badge; describe retry and restoration behavior accurately. Release linker version injection remains supported. |

Release CI now runs the race detector across every Go package rather than only the daemon package.

## Verification

Passed locally:

- `go vet ./...` and `go test -race ./...`, including new regression tests for A1–A6.
- Every top-level `scripts/test_*.py` suite: installation, updater backend/apply, startup, watchdog, cluster sync, secrets, WLAN mapping/publishing/Mac helper, and radio history.
- `scripts/ops/main-launch-current/ram-diagnostic/test_run_foreground.py` (4 tests).
- `node scripts/test_update_ui.js`, including safe rendering and asynchronous action regressions.
- `python3 scripts/check_secrets.py`.
- `sh -n` for all top-level shell scripts and the CI-required ShellCheck warning checks on `install.sh` and `start.sh`.
- Linux ARMv7 and ARM64 daemon cross-builds with CGO disabled.
- `git diff --check`.

Go was tested using the locally installed `go1.27.4 darwin/arm64`; CI still specifies Go 1.22. Writable temporary Go caches were used because the normal user caches are outside the sandbox. Cross-build success does not execute Linux raw socket or Broadcom driver paths.

## Remaining operational and design limitations

1. **Driver transactions are best effort.** Broadcom updates span separate commands and restoration itself can fail; mode/list readback cannot eliminate transient gaps. The 3-second apply/restoration deadlines need hardware timing measurements across all active VAPs. Existing ASUS deny-list ownership is not tracked separately from controller blocklists, so the controller list still replaces an existing deny list. Avoid mixing the two policy owners. Full compositional ownership requires persistent provenance and firmware-specific acceptance tests, not blindly merging lists that would prevent later unblock operations.
2. **Driver restart reconciliation is incomplete.** `MacFilterDispatcher` caches its last successful list and returns unchanged for repeats without checking for driver state lost after a wireless restart. A safe reconciliation design must detect actual driver generation/state and avoid deauthenticating all blocked clients on every heartbeat. Validate this on the target firmware before relying on controller bans as an enforcement control.
3. **Configuration acknowledgment is metadata.** `cfgversion` persists the received controller revision; the bridge does not apply arbitrary WLAN, VLAN, or radio settings. Keep ASUS/MerVLAN as configuration owners. Changing wire acknowledgment semantics without controller fixtures risks provisioning loops.
4. **Health is collection health.** Fresh telemetry does not establish functioning Wi-Fi association, DHCP, DNS, routing, or VLAN isolation. An independent wireless/data-plane probe is needed for those claims. Repeated HTTP errors preserve keys but have no exponential transport retry backoff; periodic/roaming-triggered exchanges continue.
5. **WLAN mapping remains an operational dependency.** Mapping cache accumulates last-good SSIDs and has no explicit deletion semantics; the watchdog publisher fetches configured URLs without schema validation before replacing the file, while the Go cache rejects invalid data in memory. Mapping provenance, authoritative deletion, bounded downloads, and firmware-safe atomic publication need an end-to-end contract with the external publisher.
6. **Release/toolchain coverage needs follow-up.** The current workflow triggers only on `dev` pushes and version tags, so an audit feature branch does not automatically get CI. Compiler support/security policy and the difference between CI Go 1.22 and the locally installed compiler should be reviewed before release; no claim of testing under 1.22 is made here.
7. **Physical acceptance remains necessary.** Verify valid Inform GCM replies with the deployed controller; AP-lock UI behavior; Broadcom `wl mac` formatting, readback, failures and rollback on the actual firmware; JFFS sync behavior under power interruption; watchdog BusyBox behavior; and absence of packet loss during command application. Keep the prior addon bundle available for recovery.

These limitations are explicitly retained because implementing them safely requires controller/driver fixtures, hardware access, or a broader policy ownership decision. No live deployment was attempted.
