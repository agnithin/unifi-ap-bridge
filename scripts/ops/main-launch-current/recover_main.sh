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
EXPECTED='15d34f02673b99d7fc5e8431aefc59088dd90b449874c78ec6701927fe642640'
actual=$(rtk proxy openssl dgst -sha256 "$LOCAL_SCRIPT" | rtk proxy awk '{print $NF}')
[ "$actual" = "$EXPECTED" ] || { echo 'Recovery script checksum mismatch.' >&2; exit 65; }
rtk sh -n "$LOCAL_SCRIPT"
REMOTE_COMMAND=$(rtk bypass python3 "$BUILDER" "$TEMPLATE" "$LOCAL_SCRIPT")
REMOTE_SHA=$(printf '%s' "$REMOTE_COMMAND" | rtk proxy openssl dgst -sha256 | rtk proxy awk '{print $NF}')
HANDOFF_DIR='/tmp/unifi-main-recovery-handoff-1f22343206c80bda01d576dfdb813e4a911e4568-20261026070142-recovery-2'
REMOTE_RECEIVER=$(rtk bypass python3 "$BUILDER" --handoff "$HANDOFF_DIR" "$REMOTE_SHA")
[ "${#REMOTE_RECEIVER}" -lt 1000 ] || { echo 'Remote recovery handoff receiver exceeds the reviewed command-size bound.' >&2; exit 66; }
# The receiver validates the fully transferred recovery script before running it.
printf '%s' "$REMOTE_COMMAND" | rtk proxy ssh -o BatchMode=yes -o ConnectTimeout=8 nithin@10.100.1.10 "$REMOTE_RECEIVER"
