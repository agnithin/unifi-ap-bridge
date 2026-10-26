#!/bin/sh
set -eu

[ "${ALLOW_MAIN_PRODUCTION_LAUNCH:-}" = YES ] || {
  echo 'Production hold remains active; set ALLOW_MAIN_PRODUCTION_LAUNCH=YES only after the hold is lifted.' >&2
  exit 64
}
PACKET_DIR=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
REPO_ROOT=$(CDPATH= cd -- "$PACKET_DIR/../../.." && pwd)
LOCAL_SCRIPT="$REPO_ROOT/scripts/update.sh"
TEMPLATE="$PACKET_DIR/launch.remote.template.sh"
BUILDER="$PACKET_DIR/build_command.py"
RUNNER="$PACKET_DIR/ram-diagnostic/run_foreground.py"
EXPECTED='2f2dc19723cf442a2ff5ea7ebd9e8b10b56b6a0c24ab2441eca602c14407547a'
REQUEST_ID='202610261245261754'
actual=$(rtk proxy openssl dgst -sha256 "$LOCAL_SCRIPT" | rtk proxy awk '{print $NF}')
[ "$actual" = "$EXPECTED" ] || { echo 'Local candidate script checksum mismatch.' >&2; exit 65; }
rtk sh -n "$LOCAL_SCRIPT"
REMOTE_COMMAND=$(rtk bypass python3 "$BUILDER" "$TEMPLATE" "$LOCAL_SCRIPT")
REMOTE_SHA=$(printf '%s' "$REMOTE_COMMAND" | rtk proxy openssl dgst -sha256 | rtk proxy awk '{print $NF}')
HANDOFF_DIR='/tmp/unifi-main-launch-handoff-1f22343206c80bda01d576dfdb813e4a911e4568-202610261245261754'
REMOTE_RECEIVER=$(rtk bypass python3 "$BUILDER" --handoff "$HANDOFF_DIR" "$REMOTE_SHA")
[ "${#REMOTE_RECEIVER}" -lt 1000 ] || { echo 'Remote handoff receiver exceeds the reviewed command-size bound.' >&2; exit 66; }
umask 077
RUN_DIR=$(mktemp -d "${TMPDIR:-/tmp}/unifi-main-launch-${REQUEST_ID}.XXXXXX")
chmod 700 "$RUN_DIR"
printf '%s' "$REMOTE_COMMAND" > "$RUN_DIR/launch.command"
# Keep the full SSH session foregrounded while durably retaining its status and
# private logs if the local tool yields or times out.
rtk bypass python3 "$RUNNER" \
  --result "$RUN_DIR/launch.result.json" --stdout "$RUN_DIR/launch.stdout.log" \
  --stderr "$RUN_DIR/launch.stderr.log" --stdin "$RUN_DIR/launch.command" \
  --timeout-seconds 180 -- rtk proxy ssh -o BatchMode=yes -o StrictHostKeyChecking=yes \
  -o ConnectTimeout=8 -o ConnectionAttempts=1 nithin@10.100.1.10 "$REMOTE_RECEIVER"
