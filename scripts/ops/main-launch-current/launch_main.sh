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
[ "${#REMOTE_COMMAND}" -lt 110000 ] || { echo 'Remote command exceeds the reviewed argument-size bound.' >&2; exit 66; }
rtk proxy ssh -o BatchMode=yes -o ConnectTimeout=8 nithin@10.100.1.10 "$REMOTE_COMMAND"
