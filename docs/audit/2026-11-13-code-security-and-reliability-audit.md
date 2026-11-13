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

## Reviewer guide and compatibility

Start with the trust boundary in `cmd/unifi-bridge/controller_commands.go:unpackControllerResponse` and its call in `main.go`: the adopted-session GCM check must precede response capture, command execution, and adoption-state mutation. For example, an encrypted CBC `{"_type":"reset"}` used to reach state processing; it now returns an error for an adopted AP. Bootstrap CBC remains accepted. This requires the deployed controller to use GCM after adoption; a controller that continues using CBC will stop recording successful exchanges rather than downgrade authentication.

Next review the MAC-filter transaction in `controller_commands.go`. With two interfaces, if the second interface's mode write fails after its list changed, both interfaces must be restored, including the second interface. Snapshot failures must cause zero writes. The rollback deadline is independent of the canceled mutation context, but shared by all restoration attempts; it is not a per-interface unlimited retry. ASUS allow mode (`2`) now rejects the entire transaction before any writes. Deny mode (`1`) continues to use the controller's list as the desired policy. Individual `wl` operations are not an atomic hardware transaction, and the snapshot/write interval cannot serialize unrelated ASUS driver changes.

Then review `pkg/inform/state.go`. The on-disk schema remains version 1 and paths remain `state.json` / `state.json.bak`; no manual format migration is required for a valid existing identity. On the first save there may be no previous generation to back up. Later saves retain the validated prior primary, not a second copy of the new state. A corrupt primary must not replace an existing valid backup. If both generations are invalid, startup fails rather than silently creating a new adoption identity. A recovered backup can contain an older adoption key/config revision, so controller compatibility still matters. Directory-sync errors now surface as persistence failures and use the existing pending-save retry path; they do not prove power-loss durability on JFFS.

Other intentional behavior changes:

- Inform URLs must resolve directly: HTTP 3xx responses now fail instead of being followed. Configure the final `/inform` endpoint if an existing installation depends on a redirect.
- JSON remains limited to 64 KiB and now must contain exactly one non-null object. Previously tolerated trailing content is rejected.
- `fixed_ap_available` becomes false. This changes advertised capability; it does not migrate or clear saved controller client-lock preferences.
- Syslog events are emitted only after a complete newline-terminated line. Lines larger than 64 KiB are discarded through the next newline; the follower then resumes. Rotation is detected by inode identity or truncation, and a replacement is read from its beginning.
- There is no release version bump: the source default and badge are aligned with the existing `version.txt` value of 1.2.15. CI's linker-injected version remains authoritative for release binaries.

### Regression test map

| Behavior | Test entry point | Evidence / boundary |
| --- | --- | --- |
| Complete snapshot and restoration | `cmd/unifi-bridge/controller_commands_test.go:TestMacFilterTransactionRestoresFailingInterface` | Injects snapshot, clear, set, mode, readback and cancellation faults; checks both interfaces' original policy and no premature deauthentication. Uses an in-memory driver, not firmware. |
| Allow-list preservation / ignored driver writes | `TestMacFilterRejectsAllowListAndSilentDriverFailure` in the same file | Rejects mode 2, unknown modes and a driver that silently ignores writes. |
| Adopted-session authentication | `TestControllerStateRepliesRequireGCMWhenAdopted` in the same file | Uses encrypted CBC/GCM reset packets and checks bootstrap compatibility. |
| Backup validation / safe writes | `pkg/inform/state_test.go:TestStateRecoveryValidatesEveryCandidate`, `TestStateSaveKeepsPrivatePreviousGeneration` | Exercises invalid primary/backup states, previous-generation retention, private file modes and a pre-existing predictable temp symlink. Does not simulate storage exhaustion or physical power interruption. |
| JSON envelope bounds | `pkg/inform/wire_test.go:TestWireRejectsInvalidJSONEnvelope` | Builds encrypted malformed input independently of the public packer, which otherwise rejects malformed input first. Existing known-answer vectors cover protocol compatibility. |
| Redirect refusal | `pkg/inform/client_test.go:TestInformPostDoesNotForwardRedirects` | Uses injected HTTP transport for 301/302/303/307/308 and verifies exactly one request. |
| Syslog partial writes / replacement | `pkg/telemetry/syslog_follower_test.go:TestSyslogFollowerPartialLinesAndLargerRotation` | Uses actual temporary log files and a replacement larger than the original. No router syslog service is involved. |

### Deployment acceptance and recovery

Before production rollout, retain the installed addon bundle and a protected copy of the current adoption state. On a test AP, verify successful adopted GCM heartbeats, client connectivity, actual driver list formatting, and policy readback. Inject driver failures only in a controlled environment where loss of wireless access is recoverable. Confirm state saves on the target filesystem do not leave a persistent `StateSaveError`; exercise restart/recovery without deleting adoption credentials. Inspect MAC-filter failure logs for restoration errors. Roll back through the existing known-good addon recovery procedure if controller compatibility, connectivity, or persistence regresses; avoid factory reset/re-adoption as a first response. Hardware failure injection, power-interruption testing and controller acceptance have not been performed by this PR.

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
