#!/bin/sh
set -eu
[ "${ALLOW_MAIN_RECOVERY:-}" = YES ] || {
  echo 'Recovery is guarded; set ALLOW_MAIN_RECOVERY=YES only after failure is confirmed and recovery preconditions are read-only verified.' >&2
  exit 64
}
PACKET_DIR=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
LOCAL_SCRIPT="$PACKET_DIR/recover.remote.sh"
TEMPLATE="$PACKET_DIR/recover.remote.template.sh"
BUILDER="$PACKET_DIR/build_command.py"
RUNNER="$PACKET_DIR/ram-diagnostic/run_foreground.py"
EXPECTED='5635a27505b03d72777ae3a9763f3867c567fc6beef65d2019248e964b33e9f2'
REQUEST_ID='202610261245261754'
actual=$(rtk proxy openssl dgst -sha256 "$LOCAL_SCRIPT" | rtk proxy awk '{print $NF}')
[ "$actual" = "$EXPECTED" ] || { echo 'Recovery script checksum mismatch.' >&2; exit 65; }
rtk sh -n "$LOCAL_SCRIPT"
REMOTE_COMMAND=$(rtk bypass python3 "$BUILDER" "$TEMPLATE" "$LOCAL_SCRIPT")
REMOTE_SHA=$(printf '%s' "$REMOTE_COMMAND" | rtk proxy openssl dgst -sha256 | rtk proxy awk '{print $NF}')
HANDOFF_DIR='/tmp/unifi-main-recovery-handoff-1f22343206c80bda01d576dfdb813e4a911e4568-202610261245261754-recovery'
REMOTE_RECEIVER=$(rtk bypass python3 "$BUILDER" --handoff "$HANDOFF_DIR" "$REMOTE_SHA")
[ "${#REMOTE_RECEIVER}" -lt 1000 ] || { echo 'Remote recovery handoff receiver exceeds the reviewed command-size bound.' >&2; exit 66; }
umask 077
RUN_DIR=$(mktemp -d "${TMPDIR:-/tmp}/unifi-main-recovery-${REQUEST_ID}.XXXXXX")
chmod 700 "$RUN_DIR"
printf '%s' "$REMOTE_COMMAND" > "$RUN_DIR/recovery.command"
# Recovery stays foregrounded and retains its client result and private logs.
rtk bypass python3 "$RUNNER" \
  --result "$RUN_DIR/recovery.result.json" --stdout "$RUN_DIR/recovery.stdout.log" \
  --stderr "$RUN_DIR/recovery.stderr.log" --stdin "$RUN_DIR/recovery.command" \
  --timeout-seconds 180 -- rtk proxy ssh -o BatchMode=yes -o StrictHostKeyChecking=yes \
  -o ConnectTimeout=8 -o ConnectionAttempts=1 nithin@10.100.1.10 "$REMOTE_RECEIVER"
