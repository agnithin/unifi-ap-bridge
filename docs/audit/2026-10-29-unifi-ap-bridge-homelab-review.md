# UniFi AP Bridge — homelab audit and practical review

**Date:** 2026-10-29 PDT  
**Repository:** agnithin/unifi-ap-bridge  
**Reviewed source:** dev at `5cd9ce9a49da99a43a2183595a41d852b279e0a6`  
**Intended repository path:** `docs/audit/2026-10-29-unifi-ap-bridge-homelab-review.md`  
**Scope:** Router telemetry collectors/parsers, inform/adoption, settings, updates, cluster synchronization, watchdog, WebUI, diagnostic helpers, CI, documentation, and operational simplicity.

## 1. Overall assessment

**Keep the architecture. Spend the next development cycle on reliable releases and truthful telemetry rather than additional UniFi feature parity.**

The project's useful goal is to make the existing ASUS XT8 APs visible and understandable in UniFi while ASUS remains responsible for Wi-Fi operation. A static Go daemon, local firmware commands, small JSON files and shell lifecycle scripts are appropriate for two homelab APs. A database, message queue, general orchestration framework, separate metrics platform or additional always-on agent would add little value.

The implementation already has substantial worthwhile protection: bounded commands, sample deadlines, authenticated controller command handling, complete verified update bundles, rollback recovery, preserved adoption identities, disabled-state handling, bounded histories and logs, and meaningful regression tests.

The main remaining risks are **misleading observations and release friction**, not a need for a larger platform:

1. Latest dev CI failed, and the updater follows the dev branch head even when no release was published for it.
2. Station-detail errors can be converted into apparent client departures.
3. Satellite diagnostics depend on an undeclared optional jq installation.
4. “In Sync” overstates what the status collector verifies.
5. Cached satellite measurements can still be rendered as current.
6. Adoption persistence failures are logged but not retried unless state changes again.
7. Documentation contains multiple conflicting “current” states.

No evidence from this session establishes a current router outage, compromise, or Wi-Fi disruption. This is a source/CI audit with local fixture tests, not a fresh live-router acceptance.

## 2. Evidence and boundaries

### Source and release identities

| Item | Observed state | Meaning |
|---|---|---|
| main | `c8ebaa49255778425b4a23353f83adea54b43d12` | Older baseline |
| dev | `5cd9ce9a49da99a43a2183595a41d852b279e0a6` | Reviewed snapshot; 117 commits ahead of main |
| Latest stable release | `v1.2.1` | A main merge alone does not publish a stable release |
| Latest dev CI | [Run 40730788493](https://github.com/agnithin/unifi-ap-bridge/actions/runs/40730788493) — failed | Build and publication steps skipped |
| Previous dev CI | [Run 40727106766](https://github.com/agnithin/unifi-ap-bridge/actions/runs/40727106766) — success at `73d6647` | Earlier code passed that workflow |
| Recorded deployment | AUDIT_STATE header says both APs ran `d9b6674`, v1.2.8 dev, on October 28 | Historical recorded evidence; not verified live today |

The target repository is not mounted in the available local repository workspace. Review used GitHub reads pinned to the full dev SHA. Fifty-five retrieved files were checked against their Git blob hashes before final local test results were recorded. Remaining review used the repository tree, branch comparison, release metadata and selected documentation.

“Scrapers” here means Broadcom/ASUS collectors and controller lookup helpers. Content Hub's website scrapers are a separate repository and are not covered by this report.

No production settings, APs, controller records, credentials, release tracks or services were changed. There was no fault injection against live hardware. The available approved observation registry did not expose a matching UniFi live-read tool.

## 3. Prioritized findings

Priority is relative to this homelab: **P1 = address before the next routine rollout; P2 = next small reliability pass; P3 = optional cleanup or conditional improvement.**

### F1 — Latest build failed; dev update resolution assumes branch head is published

**Priority: P1. Evidence: confirmed CI failure and source path; no live update attempted.**

The latest CI job failed in `TestPlatformLocateController`, at the assertion that LED brightness had returned to `255\n`. Both binary-build and release-publication steps were skipped.

The normal dev updater resolves `/commits/dev`, constructs `dev-<head SHA>`, and downloads that exact release's complete bundle. Therefore a failed build—or a documentation-only head whose build was skipped—leaves the normal update path targeting an unpublished generation. It safely refuses installation, but “update dev” becomes unusable until a publishable head exists.

This is particularly relevant now: the latest UI changes are source changes, not a successfully published build from this run.

**Small fix:**

- Fix Locate completion synchronization and its test; do not merely increase a sleep or bypass the failing test.
- Resolve ordinary dev updates from the newest successfully published, complete immutable dev release. Preserve exact-SHA validation after resolution.
- Alternatively, require explicit selection of the last published build and show “newer source has no published build.”
- Once accepted, publish a matching stable version tag. Keep the existing tag/version consistency check.

**Acceptance:** An unpublished dev head does not strand the updater; an installed healthy generation remains untouched. The exact intended release passes tests before publication.

**Sources:** [workflow](https://github.com/agnithin/unifi-ap-bridge/blob/5cd9ce9a49da99a43a2183595a41d852b279e0a6/.github/workflows/release.yml), [update.sh lines 693–743](https://github.com/agnithin/unifi-ap-bridge/blob/5cd9ce9a49da99a43a2183595a41d852b279e0a6/scripts/update.sh#L693), [failed job](https://github.com/agnithin/unifi-ap-bridge/actions/runs/40730788493/job/122966207409).

### F2 — The Locate implementation has a completion race

**Priority: P1 for release stability; low physical impact. Evidence: source and hosted failing test.**

`Execute(false)` cancels the blinking goroutine, writes the original brightness and returns without waiting for that goroutine to finish. The old goroutine also writes brightness during blinking and in its deferred restore. Concurrent file writes can race with the test's immediate read. Starting Locate again also cancels the prior worker without joining it, so the new worker can sample a temporary blink value as its “original” brightness, and the old worker can restore over the new one.

This supports a real lifecycle race; the exact ordering behind the recorded CI failure was not instrumented in this session.

**Small fix:** Give the blinking worker a completion channel; cancel and wait before reading/replacing state or returning from stop. Preserve the pre-Locate brightness once per active session. Keep fixed LED paths and the 15-second bound.

**Acceptance:** Repeated start/stop, start/start/stop, natural timeout and initially-off LED cases restore the correct state using deterministic completion checks.

**Sources:** [controller_commands.go](https://github.com/agnithin/unifi-ap-bridge/blob/5cd9ce9a49da99a43a2183595a41d852b279e0a6/cmd/unifi-bridge/controller_commands.go), [test](https://github.com/agnithin/unifi-ap-bridge/blob/5cd9ce9a49da99a43a2183595a41d852b279e0a6/cmd/unifi-bridge/controller_commands_test.go#L121).

### F3 — All station-detail failures are treated as departures

**Priority: P1 for telemetry correctness. Evidence: confirmed source behavior; occurrence on live APs unmeasured.**

The earlier roaming race was real: a client can leave between `assoclist` and `sta_info`. The fix now skips a station on **any** non-parent-context error or empty response. That also covers ordinary command failure and a per-command timeout while the wider sample context is still valid.

The resulting shortened station list is treated as a successful sample. It updates client counts, informs UniFi, and feeds `StationTimeline.Observe(..., true)`. A collection problem can therefore appear as a client disconnect followed by reconnect, contaminating both local history and optional cross-AP correlation.

**Small fix:**

- Skip a client only for the recognized departure response, or after one bounded recheck confirms it left the association list.
- Mark other station-detail failures as an incomplete station sample.
- Do not generate departure events from incomplete samples. Preserve last-good observations with a stale/incomplete label rather than inventing absence.

**Acceptance:** Real departure still succeeds; timeout, generic driver failure and malformed output do not create false disconnect events. One bad detail query need not take down unrelated port/system diagnostics.

**Sources:** [asus.go lines 679–738](https://github.com/agnithin/unifi-ap-bridge/blob/5cd9ce9a49da99a43a2183595a41d852b279e0a6/providers/broadcom/asus.go#L679), [main.go](https://github.com/agnithin/unifi-ap-bridge/blob/5cd9ce9a49da99a43a2183595a41d852b279e0a6/cmd/unifi-bridge/main.go#L417), [timeline.go](https://github.com/agnithin/unifi-ap-bridge/blob/5cd9ce9a49da99a43a2183595a41d852b279e0a6/pkg/telemetry/timeline.go).

### F4 — Satellite diagnostics silently depend on jq

**Priority: P1 for the new cluster UI. Evidence: source and isolated fallback reproduction.**

The new `extract_json_field` helper uses jq to obtain ports, radio statistics and system statistics. If jq is absent or unusable, it returns `[]` or `{}` silently. The new unified panels consequently lose measurements even when status JSON contains them.

This conflicts with the project's clean-firmware, minimal-dependency goal. jq availability on today's APs was not verified, so this is a portability defect, not a claim that their panels are currently empty.

**Small fix:** Reuse the existing Go binary for a tiny fixed-schema status projection, or explicitly detect the missing capability and show “satellite details unavailable.” Avoid building a general JSON parser in shell or adding a runtime package manager.

**Acceptance:** Run the cluster fixture with jq unavailable. Missing parsing support must be distinguished from an AP having no ports/radios.

**Source:** [sync_nodes.sh lines 131–137 and 489–495](https://github.com/agnithin/unifi-ap-bridge/blob/5cd9ce9a49da99a43a2183595a41d852b279e0a6/scripts/sync_nodes.sh#L131).

### F5 — “In Sync” does not establish configuration equality

**Priority: P2. Evidence: confirmed source behavior and hash-comparison reproduction.**

Routine status sets “In Sync” using health, version and commit comparisons. It does not compare the effective shared configuration. Two healthy nodes with different controller/settings values can receive this label. When Main's commit is unavailable, the routine status path can still label a healthy same-version node “In Sync.”

The new `commits_match` helper also compares only the first seven characters after unequal strings are detected—even if both inputs are complete 40-character hashes. Distinct full hashes with the same seven-character prefix returned success in an isolated reproduction. A collision is unlikely in this small repository, but weakening full identities is unnecessary.

**Small fix:** Use “Same build” for routine build agreement, and “Settings applied” for a verified operation. If configuration equality is useful, compare only a small normalized shared-settings subset, excluding node identity and learned topology. Compare full hashes exactly; permit short-prefix compatibility only when an input is genuinely abbreviated, clearly labeled as partial identity.

**Acceptance:** Same-build/different-settings does not imply equal configuration; two unequal full hashes never match.

**Source:** [sync_nodes.sh](https://github.com/agnithin/unifi-ap-bridge/blob/5cd9ce9a49da99a43a2183595a41d852b279e0a6/scripts/sync_nodes.sh#L119).

### F6 — Unified satellite measurements lose freshness context

**Priority: P2. Evidence: source inspection.**

The cluster collector includes cached ports/radio/system values even when node health is stale or stopped. The unified ports and RF renderers display those arrays without consulting node freshness. The projected node data also omits the daemon's detailed `sample_fresh`, `sample_updated_at` and `sample_stage_errors` fields.

A status banner can therefore indicate a problem while a separate panel still shows a green port or a numerical RF reading without its age. This matters during the failures the dashboard is meant to diagnose.

**Small fix:** Carry sample timestamp and relevant stage availability through the projection. Show “last observed …” and mute stale rows; keep unavailable separate from zero and disconnected. Use the same simple rendering rule for Main and Node.

**Acceptance:** With fresh, stale, partial and unreachable fixtures, only fresh measurements are presented as current.

**Sources:** [sync_nodes.sh](https://github.com/agnithin/unifi-ap-bridge/blob/5cd9ce9a49da99a43a2183595a41d852b279e0a6/scripts/sync_nodes.sh), [WebUI RF renderer](https://github.com/agnithin/unifi-ap-bridge/blob/5cd9ce9a49da99a43a2183595a41d852b279e0a6/webui/unifi-bridge.asp#L471), [ports renderer](https://github.com/agnithin/unifi-ap-bridge/blob/5cd9ce9a49da99a43a2183595a41d852b279e0a6/webui/unifi-bridge.asp#L672).

### F7 — Failed adoption persistence can remain invisible until restart

**Priority: P2. Evidence: confirmed control flow; no live storage failure induced.**

After a controller response changes adoption state, `Save()` failure produces a warning, but the exchange continues and records a successful heartbeat. If subsequent responses are ordinary noops, `changed` is false and persistence is not retried. The process can work from its in-memory key while disk still contains an old identity.

A transient full/read-only filesystem during adoption or key update can therefore leave an apparently healthy bridge that loses the accepted state at restart.

**Small fix:** Track pending persistence, retry with a modest bound/backoff during subsequent exchanges, and expose a separate “state not saved” warning. Preserve atomic replacement. Do not write unchanged state every ten seconds.

**Acceptance:** Inject one save failure, restore storage, send noop responses, and verify eventual persistence and a clean restart with the same identity.

**Sources:** [main.go lines 560–574](https://github.com/agnithin/unifi-ap-bridge/blob/5cd9ce9a49da99a43a2183595a41d852b279e0a6/cmd/unifi-bridge/main.go#L560), [state.go](https://github.com/agnithin/unifi-ap-bridge/blob/5cd9ce9a49da99a43a2183595a41d852b279e0a6/pkg/inform/state.go).

### F8 — Some client fields still assert more than the collector knows

**Priority: P2 for accurate reporting; conditional guest work. Evidence: source inspection.**

- When generation markers are missing, the station parser defaults to Wi-Fi 5 on 5 GHz and Wi-Fi 4 on 2.4 GHz. Band alone does not establish those capabilities.
- Station and VAP `is_guest` are hardcoded false.
- Missing station byte counters become numeric zero, unlike the pointer/availability treatment used for several VAP metrics.
- CCA parsing explicitly says driver sample age is unknown, while the measured value is now also mapped to UniFi `cu_interf`.

These are reporting limitations. They do not show that VLAN isolation is broken or that ordinary Wi-Fi traffic is being altered.

**Small fix:** Omit unsupported capability flags and distinguish missing counters where the wire schema permits. Implement guest classification only if actual guest/isolation configuration makes it useful; derive it from real configuration, not merely a VLAN number. Document CCA source/age limitations and compare a few readings with the controller before relying on them for decisions.

**Acceptance:** Missing fields remain unknown, true zero remains zero, and a representative guest SSID is classified correctly if that feature is enabled.

**Sources:** [sta_info.go](https://github.com/agnithin/unifi-ap-bridge/blob/5cd9ce9a49da99a43a2183595a41d852b279e0a6/providers/broadcom/sta_info.go), [asus.go](https://github.com/agnithin/unifi-ap-bridge/blob/5cd9ce9a49da99a43a2183595a41d852b279e0a6/providers/broadcom/asus.go#L601), [cca_stats.go](https://github.com/agnithin/unifi-ap-bridge/blob/5cd9ce9a49da99a43a2183595a41d852b279e0a6/providers/broadcom/cca_stats.go).

### F9 — Documentation drift is now an operational risk

**Priority: P2. Evidence: confirmed contradictory documents.**

README still describes `edfdca5`/v1.2.5 and says Locate/interference remain pending. AUDIT_STATE's current header describes `d9b6674`/v1.2.8 with both implemented. Older action tables still describe already completed work as pending. The prior full-system audit targets `c520804`/v1.2.6 and should not be treated as current.

AUDIT_STATE is approximately 299 KB. There are also multiple historical launch/recovery packet directories. These preserve useful evidence but make the next safe action difficult to locate.

**Small fix:** Keep one short current-status document with Main/Node deployed SHA, latest published SHA, tested firmware/controller versions and three next actions. Move dated history to an archive and link it. Mark historical launch packets non-routine; retain fixtures still used by tests. Distinguish “source implemented,” “published,” “deployed,” and “verified in UI.”

Avoid “60 clients undisturbed” or “zero disruption” based only on client counts. That is not a packet-loss measurement.

**Sources:** [README](https://github.com/agnithin/unifi-ap-bridge/blob/5cd9ce9a49da99a43a2183595a41d852b279e0a6/README.md), [AUDIT_STATE](https://github.com/agnithin/unifi-ap-bridge/blob/5cd9ce9a49da99a43a2183595a41d852b279e0a6/docs/AUDIT_STATE.md), [prior audit](https://github.com/agnithin/unifi-ap-bridge/blob/5cd9ce9a49da99a43a2183595a41d852b279e0a6/docs/audit/2026-10-28-unifi-ap-bridge-full-system-audit.md).

### F10 — Small lifecycle and maintenance gaps

**Priority: P3, except test coverage should accompany the next release fix.**

- `test_watchdog.py` exists and all six tests passed locally, but the release workflow does not invoke it. Add it to the existing job.
- The workflow runs ordinary Go tests but not race tests or vet. A single existing-job race/vet check is proportionate for the Locate concurrency change; no large CI matrix is needed.
- CLI `stop` kills the process without disabling configuration; the watchdog can restart it. Document “temporary stop” versus persistent disable, or make the CLI distinction explicit.
- The WLAN cache merges valid partial updates into previous entries and rejects an empty file. Removing an entry from the published map does not necessarily remove it from the running cache. Document a deliberate replacement/clear operation; retain last-good values on actual lookup failure.
- The trusted-host WLAN helper and optional roam correlator should remain on-demand unless a concrete recurring need is established. They should not become required for every inform exchange.
- Confirm durable backup coverage for each AP's effective config, adoption state, WLAN cache and necessary SSH identity, plus one known-good release. The audit did not establish whether an external ASUS/JFFS backup already covers these; this is an open check, not a proven missing backup.

## 4. What is already good — preserve it

| Area | Positive assessment |
|---|---|
| Runtime | Static Go, standard-library module, no database or agent framework required |
| Sampling | Two-second command deadlines, bounded overall sample budget, process-group cleanup, per-sample caching |
| Partial telemetry | Several optional stage failures are represented separately; improve station completeness using the same idea |
| Inform | Packet/body/decompression limits, MAC validation, GCM handling and guarded reboot dispatch |
| Updates | Complete immutable bundles, checksums, archive/member validation, transaction journal, rollback and preserved private state |
| Watchdog | Backoff, progress checks, lifecycle locking and disabled-state awareness; keep it scoped to the bridge |
| Counters | VAP interface counters and membership/reset-aware retry logic are preferable to blindly summing client lifetime values |
| Topology | Multi-bridge FDB/ARP handling and bounded learned-parent caching address real homelab needs |
| Diagnostics | Bounded private packet captures and opt-in scans; no need to pursue monitor-mode feature parity |
| Host lookups | Exact SSID matching, bounded responses, private credentials, atomic cache publication and last-good handling |
| Testing | Substantial fixture coverage of the risky lifecycle paths |

The review does not recommend undoing transactional updates or identity preservation merely to reduce line count. Those protections address failures already encountered in this project.

## 5. Simplest useful operating model

1. **ASUS owns connectivity:** SSIDs, channels, VLANs and ordinary Wi-Fi configuration stay in ASUS/Merlin.
2. **Bridge owns observation:** Clients, radio measurements, physical ports and topology go to UniFi, with explicit unknown/stale values.
3. **Optional controls stay narrow:** Locate, explicitly enabled reboot, and manual diagnostics remain fixed operations. Avoid controller firmware flashing, general shell execution or broad radio reconfiguration.
4. **Use existing monitoring:** If monitoring is added, use one existing monitor per AP/bridge with a sustained-failure threshold and recovery notification. Separate “router unreachable” from “telemetry stale.” Do not alert on each skipped station or one missed inform.
5. **Keep one release path:** Accepted immutable bundle, one AP first, verify, then the second AP. Keep a known-good rollback.
6. **Keep one current-status page:** History belongs behind links.

Do not add Kubernetes, a queue, a time-series database, a new control service, a general plugin framework, continuous RF scanning, an always-running AI agent or automatic forget/re-adopt recovery for this two-AP project.

## 6. Recommended work packages

| Order | Bounded package | Complete when |
|---|---|---|
| 1 | Locate synchronization plus published-release resolution | Exact release tests pass; unpublished source head cannot strand dev updates |
| 2 | Collector truthfulness and cluster display | Driver errors do not create disconnects; missing jq is explicit; stale data is labeled; build/config labels are accurate |
| 3 | Adoption save retry | Transient storage failure recovers without losing the accepted identity at restart |
| 4 | Documentation and release closure | One current status page, stable tag published after acceptance, backup coverage recorded |
| Optional | Guest classification and richer roam/RF detail | A real user-visible gap is demonstrated and a small fixture/live acceptance proves it |

Stop adding features until packages 1–3 are complete. Package 4 should accompany that release, not become a documentation project of its own.

## 7. Validation performed

### Local tests against pinned source

| Suite | Result |
|---|---|
| Cluster synchronization | 11 passed |
| Update backend | 13 passed |
| Update application/rollback | 35 passed |
| Startup | 10 passed |
| Installer | 12 passed |
| Watchdog | 6 passed |
| WLAN mapping resolver | 4 passed |
| WLAN mapping publication | 4 passed |
| Mac WLAN helper | 7 passed |
| **Python total** | **102 passed** |
| WebUI JavaScript regression suite | Passed |
| Shell syntax for retrieved top-level lifecycle scripts | Passed |

An initial updater-fixture hash mismatch came from a transport-added trailing newline in the temporary audit copy. The copies were normalized and verified against Git blob hashes, and the updater suite then passed all 35 tests. This was not counted as a repository defect.

### Additional checks

- Isolated `commits_match` evaluation accepted unequal full identities sharing the seven-character prefix.
- Isolated unavailable/unusable-jq evaluation returned empty ports for populated input.
- Reviewed the actual hosted failed job log; failure is `TestPlatformLocateController`.
- Go was unavailable in this execution environment; no local Go/race/vet pass is claimed.
- Fixture tests use simulated firmware commands. They do not establish BusyBox/Dropbear behavior or LED behavior on live XT8 firmware today.
- No new live tests of update, reboot, credential rotation, packet capture, scanning, fault recovery or network performance were performed.

## 8. Minimal acceptance before calling the next release done

- Exact release commit passes CI, including deterministic Locate tests and existing watchdog tests.
- One AP canary reports the expected full build identity, fresh sample, successful inform and truthful cluster details.
- An unchanged settings save performs no unnecessary restart.
- Disabled state remains disabled.
- Identity/config/cache survive the normal update path.
- A fixture covers interrupted update recovery; do not induce a live outage just to demonstrate it.
- Controller outage and malformed station-detail fixtures cannot produce misleading healthy/empty results.
- The second AP is updated only after the first is accepted.
- Stable tag and current-status page identify the same accepted generation.

This is a small stabilization release, not a redesign.
