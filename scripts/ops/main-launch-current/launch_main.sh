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
EXPECTED='2f2dc19723cf442a2ff5ea7ebd9e8b10b56b6a0c24ab2441eca602c14407547a'
actual=$(rtk proxy openssl dgst -sha256 "$LOCAL_SCRIPT" | rtk proxy awk '{print $NF}')
[ "$actual" = "$EXPECTED" ] || { echo 'Local candidate script checksum mismatch.' >&2; exit 65; }
rtk sh -n "$LOCAL_SCRIPT"
REMOTE_COMMAND=$(rtk bypass python3 "$BUILDER" "$TEMPLATE" "$LOCAL_SCRIPT")
REMOTE_SHA=$(printf '%s' "$REMOTE_COMMAND" | rtk proxy openssl dgst -sha256 | rtk proxy awk '{print $NF}')
HANDOFF_DIR='/tmp/unifi-main-launch-handoff-1f22343206c80bda01d576dfdb813e4a911e4568-20261026054615'
REMOTE_RECEIVER=$(rtk bypass python3 "$BUILDER" --handoff "$HANDOFF_DIR" "$REMOTE_SHA")
[ "${#REMOTE_RECEIVER}" -lt 1000 ] || { echo 'Remote handoff receiver exceeds the reviewed command-size bound.' >&2; exit 66; }
# Keep the SSH exec request small. The receiver reads the complete stream into a
# private file, validates its exact SHA-256 and syntax, then runs it in-session.
printf '%s' "$REMOTE_COMMAND" | rtk proxy ssh -o BatchMode=yes -o ConnectTimeout=8 nithin@10.100.1.10 "$REMOTE_RECEIVER"
