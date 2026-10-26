# Main `update.sh` RAM-only diagnostic packet

**The reviewed RAM-only diagnostic was executed once on Main under the approval recorded in `docs/AUDIT_STATE.md`; production remains held.** The packet does not invoke a launch/recovery wrapper or candidate asset. Its only executable remote payload is `diagnose.remote.sh`; copied updater bytes and every extracted candidate member remain data.

The historical `! cmp -s` check established only a nonzero comparison result. Later reads established that two stage paths are absent now. Neither observation establishes whether the file was different, absent, or unreadable at the earlier comparison, whether extraction initially created it, or who removed it. This experiment can locate a disappearance within a new 60-second observation window, but cannot identify the actor.

## Pinned inputs and local packet construction

The candidate is immutable at `1f22343206c80bda01d576dfdb813e4a911e4568` (ARMv7). The archive SHA-256 is `16300c4bc0f08f6d5bfadae8427a7928f01de1470eaf7d000c317be6d27657f1`; the sidecar SHA-256 is `113a1ffc283a5a27ba1594736d827088aacab9e7936478234e0c775677eaf48f`; the updater SHA-256 is `2f2dc19723cf442a2ff5ea7ebd9e8b10b56b6a0c24ab2441eca602c14407547a`. The local builder checks these values, sidecar contents, the archive's exact 11 regular-file member set and manifest updater bytes before making a private local stream package.

After a separate approval for remote execution, construct a fresh packet locally:

```sh
rtk python3 scripts/ops/main-launch-current/ram-diagnostic/prepare_packet.py \
  --archive /path/to/pinned-armv7.tar.gz \
  --sidecar /path/to/pinned-armv7.tar.gz.sha256 \
  --updater scripts/update.sh \
  --output /tmp/unifi-main-ramdiag-local-UNIQUE/payload.tar.gz
```

The builder generates a fresh numeric request ID, writes a mode-0600 local payload and a mode-0600 `*.receiver.command`, and does not contact a host. Its generated single-session remote receiver command checks Main identity, daemon PID, a status timestamp no more than 45 seconds old, the expected recovered commit `be106ec33fe6790af30a600d11bd05246445c530`, adopted/running/GCM/Active status, and at least 20 MiB free in `/tmp`. It rejects an existing stage and verifies the transferred outer-payload checksum before extraction. The exact remote invocation, when separately authorized, is:

```sh
REMOTE_COMMAND=$(rtk cat /tmp/unifi-main-ramdiag-local-UNIQUE/payload.tar.gz.receiver.command) && \
rtk proxy ssh -o BatchMode=yes -o ConnectTimeout=8 nithin@10.100.1.10 "$REMOTE_COMMAND" \
  < /tmp/unifi-main-ramdiag-local-UNIQUE/payload.tar.gz
```

The command above documents the receiver invocation shape; the executed instance used a freshly generated request ID, local payload and remote stage recorded in `docs/AUDIT_STATE.md`. No old stage was reused. A future approved run must use a fresh output name and candidate artifact path. The SSH session streams one outer archive to Main; it does not use the launch/recovery handoff or run either wrapper. If a future session reaches 180 seconds, stop waiting and do not reconnect or retry. Treat whether the remote foreground script finished as unknown; preserve the stage and obtain a read-only review before any further action.

## Remote phases, writes and bounds

The generated receiver checks the exact hostname `ZenWiFi_XT8-7B10`, exactly one `unifi-bridge` PID, matching PID and recent sanitized daemon status. It does not print the status file or inform contents. The receiver checks `/tmp` free space (20 MiB minimum), rejects an existing stage, then creates only `/tmp/unifi-main-ramdiag-<candidate>-<request-id>` with umask 077 and mode 0700. It writes `packet.tar.gz`, verifies its SHA-256, and extracts four reviewed payload files under the private `payload/` directory. The remote diagnostic repeats baseline health and pinned artifact/updater checksum checks before creating test cases.

The four independent cases are:

1. `case-updater/update.sh`: exact pinned updater bytes copied with `cp -p`.
2. `case-neutral/payload.bin`: the same bytes and mode under a neutral basename.
3. `case-control/update.sh`: a short, harmless text file in a different subdirectory; it is never invoked.
4. `case-extract/`: the pinned archive extracted as data into an otherwise empty directory; no extracted file is invoked.

The remote writes are limited to the fresh private `/tmp` stage: outer payload, payload files from the verified tar stream, four case directories, two updater-byte copies, the control text file, candidate extraction files, `observations.log`, and a short-lived overwritten `observe.list` used to distinguish a confirmed absent basename from an unlistable parent. No `/jffs`, installed asset, runtime, config, key, watchdog, lock, journal, controller setting, or Node path is written or read for this test. For baseline health only, the receiver and diagnostic read allowlisted fields from the existing `/tmp/var/wwwext/unifi-bridge/status.js`; they do not interact with a UI page or write a UI asset. No chmod, background job, lock, cleanup or tracing setup occurs. The original retained launch stages are not inspected, deleted, or recreated.

Observations are written immediately after each file creation/extraction, then at elapsed samples 1, 5, 15, 30 and 60 seconds. Each row contains only an epoch timestamp, fixed case label, sample, status, type, `ls -lnd` mode, byte count and SHA-256. Regular-file read/hash errors are `read_error`; inability to list/inspect is `observation_error`; `absent_confirmed` is reported only when the parent is listable and the basename is not listed. Symlinks are identified without reading their target. No file contents are logged. The expected bound is 24 observation rows plus one extraction result, under 8 KiB. The private stage and all evidence are retained; nothing auto-cleans or recreates a missing file.

Measured inputs are 2,344,402 compressed archive bytes and 5,832,906 regular-file extraction bytes. Together with the retained outer payload, two updater copies and metadata, expected use is below 12 MiB; the 20 MiB free-space preflight leaves a reserve. The test runs in one foreground SSH session, with 60 seconds of deliberate observation and a 180-second operator wall-clock stop. Main's BusyBox was previously observed as 1.25.1; this packet uses its `/bin/sh`, `hostname`, `pidof`, `date`, `sed`, `grep`, `head`, `df`, `mkdir`, `cat`, `openssl`, `awk`, `tr`, `cp`, `wc`, `ls`, `tar`, and `sleep` tools, including `tar -xzpf` used by the reviewed recovery. It deliberately avoids relying on `stat`, `readlink`, `find`, `timeout`, or `chmod`. Local shell syntax/fixtures do not claim to prove a command exists on the current firmware; verify the already installed BusyBox applet list read-only before any later approval, and stop on a missing applet.

Stop before test-case creation on any identity, process, health, freshness, space, checksum, archive-member, or extraction error. Do not delete anything, automatically rebuild a missing test file, start another session, or invoke recovery. If a case becomes unobservable, preserve the `observation_error` row and stop interpretation rather than treating it as absent.

## How to interpret outcomes

| Observation | What it supports | What it cannot establish |
|---|---|---|
| All four remain regular with stable hashes for 60 seconds | No disappearance occurred in these paths during this sample window | Why historical `update.sh` was absent; behavior during the updater or another time |
| Updater-named copy disappears while neutral copy, control `update.sh`, and extracted updater remain | A trigger associated with that specific pathname or directory becomes more plausible | A filename rule, cleanup code, or actor; timing alone does not identify any of these |
| Both updater-byte copies disappear while control/extracted files remain | A content-, source-, or shared-directory-related trigger becomes more plausible | Exact trigger or actor |
| Both files named `update.sh` disappear while neutral file remains | A basename-related trigger becomes more plausible | That any process specifically targets update scripts, or who did it |
| Extracted updater is absent immediately after tar reports success while its pinned member is present | Extraction outcome/path behavior needs investigation | That another process removed it; verify the tar exit/member evidence and stop |
| Archive member check, extraction, metadata, listing, or hashing errors | The observation is inconclusive at the failing phase | File absence or a removal event |
| A file is confirmed absent after an earlier successful immediate sample | Disappearance occurred between those two samples | Which process/user caused it or the exact instant within the interval |

Regardless of outcome, this diagnostic does not establish the historical actor. It cannot reproduce the updater's execution-time environment because that updater is deliberately never run. Production remains held and the previous approval is consumed.

## Local validation

`verify_local.py` extracts the sanitized observation helper and exercises regular file, symlink, confirmed absence, missing/unlistable parent, read/hash error, and updater-disappears/neutral-remains fixtures. It asserts that log rows contain no file contents and guards against candidate invocation or forbidden paths. `prepare_packet.py` can be run with the exact pinned local archive to validate candidate identity and generate a local payload; neither local check sends remote commands.

## Local updater lifecycle regression

`verify_candidate_lifecycle.py --archive PATH --sidecar PATH` checks the immutable candidate archive and runs the existing updater success and missing-stage regressions in disposable firmware fixtures. The successful case loads the exact pinned `update.sh` member and includes that updater as the candidate generation's `update.sh`. It traces bounded `tar`, `cp`, `mv`, and `rm` command boundaries plus pre/post observations of the launch-stage script. The log records only fixture-relative paths, state, size, mode, SHA-256 and command result; it never records file contents. The production candidate-comparison helper then verifies `update.sh` content and mode using the same separate classifications as launch postflight.

The local host cannot execute the pinned ARMv7 daemon. The lifecycle fixture therefore substitutes a harmless synthetic daemon for runtime checks while retaining the exact candidate updater bytes and the exact candidate updater file. This validates updater orchestration and `update.sh` file handling; it does not prove execution of the complete ARMv7 archive on router firmware.

`test_run_foreground.py` checks `run_foreground.py`, a local-only command wrapper for future approved foreground SSH sessions. It atomically retains a mode-0600 JSON result with the actual client exit code and total monotonic elapsed time, plus private stdout/stderr files. It records a timeout as `timed_out_remote_completion_unknown` and never retries. Example shape:

A child terminated by a signal is recorded with its raw negative subprocess code (for example, `-15`), a `terminated_by_signal_remote_completion_unknown` status and the signal number. The wrapper returns the shell-compatible nonzero status `128 + signal`; neither interruption nor timeout establishes remote completion.

```sh
rtk python3 scripts/ops/main-launch-current/ram-diagnostic/run_foreground.py \
  --result /private/tmp/private-run/result.json \
  --stdout /private/tmp/private-run/stdout.log \
  --stderr /private/tmp/private-run/stderr.log \
  --stdin /private/tmp/private-run/payload.tar.gz \
  --timeout-seconds 180 -- rtk proxy ssh -o BatchMode=yes \
  -o StrictHostKeyChecking=yes -o ConnectTimeout=8 \
  -o ConnectionAttempts=1 nithin@10.100.1.10 REMOTE_COMMAND
```

The command example is documentation only. No AP access or remote execution is authorized by this local lifecycle regression.

## Instrumented Main-only launch packet (prepared for review; not executed)

The new packet uses request ID `202610261245261754` and stage `/tmp/unifi-main-launch-1f22343206c80bda01d576dfdb813e4a911e4568-202610261245261754`. It leaves all earlier launch, recovery and RAM-diagnostic stages untouched. Candidate commit remains `1f22343206c80bda01d576dfdb813e4a911e4568`; the archive SHA-256 remains `16300c4bc0f08f6d5bfadae8427a7928f01de1470eaf7d000c317be6d27657f1`, the downloaded release-sidecar SHA-256 remains `113a1ffc283a5a27ba1594736d827088aacab9e7936478234e0c775677eaf48f`, and updater SHA-256 remains `2f2dc19723cf442a2ff5ea7ebd9e8b10b56b6a0c24ab2441eca602c14407547a`. Separately, the updater writes a normalized installed sidecar whose SHA-256 is `0e230741aefdf01fdff06eb291693299edcaeddc9ed7e45fb646593f2252116a`; its sole row must contain the pinned archive digest and filename `unifi-bridge.tar.gz`. The launch verifies the pinned updater before handoff, then checks the installed archive hash and both the normalized sidecar hash and exact digest/filename row after updater return. Candidate acceptance gates remain fail-closed.

The launch records at most 32 sanitized observation rows (under 8 KiB), with fixed phase and file labels, state/type, mode, size and SHA-256. It samples the staged updater after same-session handoff, staged and installed updater after updater return, extracted updater immediately after tar extraction even on extraction failure, extracted and installed updater before and after their content/mode comparison, and all three paths before acceptance. `candidate-files.status` still retains the existing exact classifications: missing, unreadable, comparison error, content difference and mode difference (plus its existing non-regular and mode-error states). The observation log never records file contents. A metadata read or log failure stops the launch. The outer packet and stage paths include the fresh request ID; the remote launch also requires the stage not to exist and at least 65,536 KiB free in `/tmp` before creating it.

The reviewed foreground runner stores the SSH client exit status (including a raw negative signal code), total elapsed time, and private stdout/stderr logs. Both launch and recovery have a 180-second foreground bound, verified host keys, batch mode, one connection attempt and an 8-second connect timeout. A timeout or signal marks remote completion unknown: retain all local/remote evidence, do not retry, and use read-only triage. The measured archive is 2,344,402 bytes compressed and 5,832,906 bytes extracted; live free space must be checked again by preflight. Snapshots and updater temporary files consume additional space, covered by the 65,536 KiB minimum gate.

The unchanged launch gates require Main identity, a sole daemon PID, fresh prior-build status and successful GCM inform, the configured effective state path, watchdog and clean journal/lock state, and exact prior-generation baselines before updater invocation. Acceptance requires the pinned candidate version/commit and fresh post-restart GCM inform, watchdog, clean transaction/locks, and preserved config/state identity and bytes/modes plus SSH-key bytes/mode. Any failed or ambiguous gate stops acceptance; no gate was weakened. Node remains outside scope and unchanged.

The concrete guarded commands that would be included in one Main-only launch authorization are:

```sh
ALLOW_MAIN_PRODUCTION_LAUNCH=YES rtk sh scripts/ops/main-launch-current/launch_main.sh
```

The accompanying bounded recovery operation, only after a failed acceptance and read-only confirmation that recovery preconditions hold, is:

```sh
ALLOW_MAIN_RECOVERY=YES rtk sh scripts/ops/main-launch-current/recover_main.sh
```

Recovery is pinned to the fresh stage above. The recovery script SHA-256 is `5635a27505b03d72777ae3a9763f3867c567fc6beef65d2019248e964b33e9f2`; the packet verifies that hash and syntax, requires Main hostname, retained prior snapshots with exact members and known-good asset hashes, unchanged protected config/state identity/SSH key and modes, no transaction journal or locks, and verifies the restored prior runtime's fresh successful GCM inform, watchdog and transaction cleanup. Any failed guard stops and retains evidence. Recovery is named in the proposed authorization but is not authorized or executed now.

Neither command has been run. No remote staging, service restart, AP mutation, controller write, Node access or cleanup occurred. Production remains held pending Sol review of this instrumented diff and a separate explicit decision.
