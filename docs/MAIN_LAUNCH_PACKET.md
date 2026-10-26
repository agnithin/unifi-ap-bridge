# Main-only launch and recovery packet

**Disposition: ready for lifting the production hold; not deployed.** This packet prepares one guarded migration of Main only. The hold remains active. Neither launch nor recovery entry point was run against either AP, and Node was not updated.

## Pinned candidate and Main baseline

- Candidate: `dev`, version `1.2.5`, source commit `a25bb6b272e6dcbe938b61fbca8d17044c6ff07d`.
- Candidate `scripts/update.sh` SHA-256: `2f2dc19723cf442a2ff5ea7ebd9e8b10b56b6a0c24ab2441eca602c14407547a`. The tracked source at that commit and the local candidate source match.
- ARMv7 Main bundle SHA-256: `cdd11384e69da61775b75fe43b4b9d48bba54b4984f39c516f757128e23422a5`.
- ARM64 companion bundle SHA-256: `279734369557ae812f6a909755c93789e028c78ae20dd8bc868ccdaee54ac757`.
- Both checksum sidecars, metadata, exact member lists, embedded updater hashes, and the ARMv7/ARM64 architecture labels were checked. The hosted build/release runs are recorded in Section 15.
- Main host identity: `ZenWiFi_XT8-7B10`. At the read-only preflight sample it ran prior daemon `be106ec33fe6790af30a600d11bd05246445c530`, version `1.2.5`, adopted with GCM and a fresh successful inform. No transaction journal or update/mutation/start lock was present; the watchdog cron was present.
- The running daemon uses `/jffs/addons/unifi-bridge/config/config.json`; that file names `/jffs/addons/unifi-bridge/state.json` as `state_path`, and the process has no `-state` override. The state file, config, and cluster key were present with mode `0600`; their contents and secret-derived fingerprints were not printed.
- Main is still a mixed compatibility generation. The prelaunch managed-asset snapshot and runtime are in a private local directory, `/private/tmp/unifi-main-launch-packet-20261026/`, with SHA-256 values `66bf50796f0ae6bb0eb662d88843bf3718fbf21a15978a85241c3695b165e01c` and `ab2ed5d03f37e3cb6cf1595f4fe9fe07d24f38dbc2f315abaa529f9c378d50ae`. The captured installed prior archive is `8199dc2446929de29f4a56e8acb4338626724a384a57fc33b6c86e1ef6d9d0bc`. These local recovery snapshots exclude config, state, and SSH keys. At launch, the remote script makes a fresh same-device snapshot and pins each prior managed file/runtime to the reviewed hashes in the launch and recovery scripts.

## Packet files and guarded commands

The durable packet is under [`scripts/ops/main-launch`](../scripts/ops/main-launch/):

- [`launch_main.sh`](../scripts/ops/main-launch/launch_main.sh) builds the command from the pinned tracked updater and [`launch.remote.template.sh`](../scripts/ops/main-launch/launch.remote.template.sh), then uses direct `rtk proxy ssh` to Main.
- [`recover_main.sh`](../scripts/ops/main-launch/recover_main.sh) sends the pinned [`recover.remote.sh`](../scripts/ops/main-launch/recover.remote.sh) through its same-session [`recover.remote.template.sh`](../scripts/ops/main-launch/recover.remote.template.sh) handoff.
- [`build_command.py`](../scripts/ops/main-launch/build_command.py) applies POSIX single-quote encoding for the validated `printf '%s'` handoff.
- [`verify_local.py`](../scripts/ops/main-launch/verify_local.py) checks commit/source and artifact hashes, sidecars, metadata/member sets, generated command syntax/size, a harmless local quote-containing sentinel roundtrip, and synthetic preservation-comparison regressions.

After the production hold is explicitly lifted, the one launch command is:

```sh
rtk proxy env ALLOW_MAIN_PRODUCTION_LAUNCH=YES sh scripts/ops/main-launch/launch_main.sh
```

The script refuses to run without the guard. It connects only to `nithin@10.100.1.10`, checks the exact hostname and baseline, creates a unique private stage at `/tmp/unifi-main-launch-a25bb6b272e6dcbe938b61fbca8d17044c6ff07d-20261026011541`, writes the updater with quoted `printf`, verifies its absolute path, OpenSSL SHA-256 and shell syntax, then invokes that same absolute path in the foreground in the same SSH command. The request ID is `20261026011541`. `apply-exact` is the receiver-side updater mode and does not propagate an update to Node.

The matching recovery command, only after a failed acceptance and after read-only checks establish the no-journal/no-lock recovery preconditions, is:

```sh
rtk proxy env ALLOW_MAIN_RECOVERY=YES sh scripts/ops/main-launch/recover_main.sh
```

This recovery entry point is guarded independently. It refuses to mutate when a transaction journal or any updater lock exists, when prelaunch snapshots do not match their pinned known-good managed assets/runtime, when protected config/key/state identity or permissions differ, or when daemon PID state is ambiguous. It acquires update, mutation, and start locks in order, rechecks journal absence and protected data while holding those locks, stops only a single identified bridge PID, atomically restores managed addon files and the prior runtime from the verified snapshot, then requires a fresh successful prior-generation GCM inform, watchdog cron, protected-data checks and lock/journal cleanup. Config, state, and SSH key files are never restored or written by this helper.

## Preservation, evidence, and failure handling

The launch preflight rechecks host, daemon identity/freshness, effective config/state paths, current prior mixed-generation asset/runtime hashes, expected protected-file modes, watchdog cron, and absence of journals/locks. It stores byte-exact private copies of config and cluster key, a private copy of the effective state file, selected normalized state identity values (`schema`, `mac`, `adopted`, `mode`, `authkey`, `paused`), file owner/group/mode signatures, prior successful inform, all managed addon assets, and runtime. The key and state copies remain mode `0600` inside the private stage. It does not print contents or fingerprints.

After the updater returns, the script retains `apply.log`, `worker.status`, safe status excerpts, candidate extraction/member evidence and all prelaunch snapshots. It verifies the exact installed ARMv7 bundle/checksum, metadata/member set, managed file contents/modes, runtime binary, candidate version/commit, a fresh active GCM inform, watchdog cron, config/key bytes and modes, state identity and mode, and transaction cleanup. Full state-file byte equality is reported separately because the daemon may legitimately update controller metadata; the acceptance requirement is continuity of the selected identity values and permissions. A difference in those values or a failed preservation read is a failed acceptance.

Preflight mismatch stops before updater invocation. A nonzero result, missing stage file, failed comparison, unexpected status, or SSH transport loss after invocation leaves the logs and stage in place and stops rollout. After reconnect, read-only inspection must establish worker/process state, journal/locks, installed generation, runtime identity, and fresh inform. A transport error alone is not proof that no update work ran. Do not clean evidence or retry the launch. If a journal or any lock remains, both guarded entry points stop; preserve evidence and use a separately reviewed journal/lock-specific reconciliation. Do not invoke generic recovery, delete locks, reboot, or retry. Use the no-journal recovery command only after its preconditions pass. If recovery stops or cannot prove the prior runtime/inform, keep Main stopped from further mutation and preserve all evidence for a new bounded review.

A successful packet run still needs the independent acceptance checks in Section 15: radios on channels 6/36/161, Main uplink to gateway Port 1 at 2.5 GbE, Node still healthy without update, and any browser-dependent Asus/UniFi checks available. The existing read-only telemetry sample confirmed those radio channels and Main Port 1 at 2.5 GbE before deployment; it does not replace a post-deployment check. Browser acceptance remains pending because no browser surface was available in this session.

## Validation performed for this packet

- `sh -n` passed for both remote scripts, both remote templates, and both guarded local wrappers.
- `verify_local.py` passed: candidate commit/source SHA, both bundle sidecars/checksums and metadata/member sets, embedded updater SHA, generated launch command (66,860 bytes) and recovery command (15,992 bytes) syntax/size, local harmless same-session `printf` roundtrip, and same/changed/error comparison plus state-identity fixtures.
- Both guarded wrappers were invoked without their guards and refused execution before SSH.
- A bounded `gpt-6.1-sol` review initially identified recovery precondition races, retry-hostile temp paths, incomplete protected-file mode checks, ambiguous state comparison, PID concatenation, and a recovery inform check that was not restart-local. The packet was updated to address these findings, use explicit permission-preserving tar extraction, and require a fresh inform associated with the prior runtime's single live PID. Final bounded re-review returned PASS with no remaining P1/P2 findings.
- This packet does not reconstruct the historical missing-stage actor or resolve the historical `state_unchanged=no` baseline/comparator. Section 15 remains authoritative for those limits.


## Separate current-fix rollout packet (not a retarget of W7)

**Disposition: locally verified and published, production held, not deployed.** The original reviewed W7 packet and its exact `a25bb6b272e6dcbe938b61fbca8d17044c6ff07d` candidate remain unchanged. This separate packet prepares one later Main-only rollout to the current source candidate after the production hold is lifted. It avoids doing an `a25bb6b` migration and then a second Main restart solely to deploy follow-up fixes. The current candidate contains the reviewed migration/recovery source plus the subsequent W9–W12 fixes, CI maintenance, the published operating-bandwidth field from `02454dfec818031cdf406753398366f49adbe1ed`, and the small settings-reader diagnostic fix. The updater bytes are unchanged across both packets (`2f2dc19723cf442a2ff5ea7ebd9e8b10b56b6a0c24ab2441eca602c14407547a`).

The exact candidate is dev commit [`1f22343206c80bda01d576dfdb813e4a911e4568`](https://github.com/agnithin/unifi-ap-bridge/commit/1f22343206c80bda01d576dfdb813e4a911e4568), version `1.2.5`, architecture `armv7`. Hosted run [40275750890](https://github.com/agnithin/unifi-ap-bridge/actions/runs/40275750890) passed and published release [`dev-1f22343206c80bda01d576dfdb813e4a911e4568`](https://github.com/agnithin/unifi-ap-bridge/releases/tag/dev-1f22343206c80bda01d576dfdb813e4a911e4568).

- Main bundle: `unifi-bridge-addon-1f22343206c80bda01d576dfdb813e4a911e4568-armv7.tar.gz`; SHA-256 `16300c4bc0f08f6d5bfadae8427a7928f01de1470eaf7d000c317be6d27657f1`.
- Release sidecar asset SHA-256: `113a1ffc283a5a27ba1594736d827088aacab9e7936478234e0c775677eaf48f`; its contents name the archive and repeat the archive checksum.
- Embedded ARMv7 executable SHA-256: `adf321be93d70c24c680a8bdad3b66eae95e32a883110928ab09bb706157c1d6`.
- The packet verifier checks the published sidecar and archive checksums, candidate/version/architecture, exact archive members and every manifest member hash, including the binary and updater. It also checks the source commit's updater hash.

The separate guarded scripts and verifier are in [`scripts/ops/main-launch-current`](../scripts/ops/main-launch-current/). The launch entry point is `launch_main.sh`; its local `ALLOW_MAIN_PRODUCTION_LAUNCH=YES` guard is intentionally still required. The actual command, after a separate hold-lift decision, is:

```sh
rtk proxy env ALLOW_MAIN_PRODUCTION_LAUNCH=YES sh scripts/ops/main-launch-current/launch_main.sh
```

The replacement packet uses a fresh stage/request ID `20261026064831`; the failed preflight's stage and handoff remain retained under `20261026054615`. The generated remote launch script is streamed on stdin over one SSH connection to a short fixed receiver command. The receiver creates a private mode-0700 directory, writes the complete stream to a file, verifies the exact streamed-script SHA-256 and shell syntax, then runs that absolute file in the foreground in the same SSH session. A truncated or altered stream fails checksum validation before the launch script can run and leaves its private handoff for diagnosis; successful handoff cleanup occurs only after the launch script returns. The launch script writes the updater with POSIX-quoted `printf` to the reviewed absolute Main stage path, applies mode `0700`, checks the pinned updater SHA-256 and `sh -n`, and invokes that same absolute path in the foreground. It calls receiver-side `apply-exact` with dev and the full immutable commit, so it does not propagate an update to Node or create an unmanaged background transfer. This stdin protocol keeps the large script out of the SSH exec argument, whose remote limit was reproduced with harmless 16 KB/68 KB commands; 8 KB exec and 68 KB stdin sentinels succeeded. The local verifier checks streamed payload and receiver syntax/size, validates quote-containing same-session execution, and rejects a truncated payload before executing it. A lost SSH result is ambiguous and must be reconciled read-only before any further action.

At the actual launch, Main must still pass exact hostname, daemon PID/runtime, fresh status and GCM health, no `-state` override, config-selected effective state path `/jffs/addons/unifi-bridge/state.json`, expected known-good mixed assets/runtime, watchdog cron, permissions, and no journal or update/mutation/start lock. Before invoking the updater, it creates a fresh mode-0700 same-device evidence directory and mode-0600 copies of config, effective state, cluster key, state identity fields and file metadata, prior managed assets and runtime. It hashes the recovery archives. This uses a fresh on-Main baseline; the old temporary CT107 backup is neither required nor presumed available. It never prints config/key/state values or secret fingerprints and never restores those protected files.

After update, the remote gate verifies the exact archive checksum/sidecar, architecture and member set, installed managed file contents/modes, binary/runtime commit/version, and a successful GCM inform from the sole live candidate PID. It orders the numeric UNIX `status_updated_at` against shell `date +%s`; the daemon's zone-less `last_successful_inform` is treated as opaque evidence and is never compared lexically with shell wall time. The status epoch must be strictly later than the updater start, no more than 45 seconds old, and carry the expected PID/commit, nonempty successful-inform value and `Active (GCM)` status. An equal-second observation is ambiguous and fails closed. The launch preflight also checks status freshness in epoch seconds and requires the status PID to match the running prior daemon. The gate checks watchdog cron, config bytes/mode, effective-state identity/mode (full state bytes are reported separately because controller metadata may change), cluster-key bytes/mode, transaction-journal absence, and absence of all three locks. It retains logs and snapshots in the stage. Recovery applies the same epoch-basis and equal-second rule when verifying the restored prior runtime's fresh inform. The post-deployment operator acceptance must separately recheck Main's gateway Port 1 at 2.5 GbE, exactly the three physical radios at 6/20, 36/40 and 161/80 MHz, fresh GCM informs and status, watchdog, cleanup, and Node health/runtime unchanged. The UI's upper-band 80 MHz rendering and three-radio view are not accepted until verified live; no channel, width, SSID, power, VLAN or other radio/network setting is changed by this packet.

The recovery entry point is independently guarded:

```sh
rtk proxy env ALLOW_MAIN_RECOVERY=YES sh scripts/ops/main-launch-current/recover_main.sh
```

The pinned recovery script `scripts/ops/main-launch-current/recover.remote.sh` has SHA-256 `2fa83e1743726c58a1b49cb321e8c58862aac16566e953493d95407796ad00ba`; its wrapper, receiver template and local verifier must all agree.

Use it only after a failed acceptance and a read-only reconnect confirms the exact Main identity, stage snapshots/checksums, no journal, no lock, and unchanged config/key/state identity and permissions. Its small SSH receiver first streams the complete pinned recovery script into a private directory, validates the generated script's SHA-256 and syntax, and only then runs it in the same session. It atomically restores only the known-good managed addon assets and `be106ec` runtime under update/mutation/start locks, then starts that prior daemon and requires a fresh GCM inform, watchdog and clean journal/locks. If a journal/lock is present, the remote stage is missing/ambiguous, protected preservation differs, or recovery cannot verify its postconditions, it stops and preserves all evidence; no cleanup, blind retry, reboot, Node action or generic recovery is authorized.

### Local packet verification

`python3 scripts/ops/main-launch-current/verify_local.py <armv7-bundle> <sidecar>` passed against the downloaded exact release assets. It verified the archive (`16300c4…d27657f1`), sidecar (`113a1f…7eaf48f`), all manifest member hashes and the exact 11-member ARMv7 package. Generated launch/recovery streams passed shell syntax, their SSH receiver commands remained below 1 KB, and same-session stdin handoff plus truncated-stream rejection, state-identity, preservation comparator, lock-cleanup and fresh-inform regressions passed. The original W7 `verify_local.py` also passed unchanged. The verifier does not contact or modify either AP.

**Consolidated production decision recommended for the later hold-lift step:** if authorizing a Main migration, approve this single exact `1f223432…` packet as the replacement *deployment choice* for the still-undeployed W7 `a25bb6b` packet, leaving the original W7 packet intact as its historical reviewed candidate. Do not deploy `a25bb6b` first and then repeat the migration for `1f223432`. This is a prepared recommendation only: the current production hold remains active, and no launch or recovery script has been run against an AP. Node remains unchanged.
