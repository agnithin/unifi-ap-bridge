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
