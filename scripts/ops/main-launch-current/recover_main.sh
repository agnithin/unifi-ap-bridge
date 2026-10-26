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
EXPECTED='33c26d45a8cc05df015625e3aab42c29857cf09459a3dd4d3e7548ca31f70f9c'
actual=$(rtk proxy openssl dgst -sha256 "$LOCAL_SCRIPT" | rtk proxy awk '{print $NF}')
[ "$actual" = "$EXPECTED" ] || { echo 'Recovery script checksum mismatch.' >&2; exit 65; }
rtk sh -n "$LOCAL_SCRIPT"
REMOTE_COMMAND=$(rtk bypass python3 "$BUILDER" "$TEMPLATE" "$LOCAL_SCRIPT")
[ "${#REMOTE_COMMAND}" -lt 110000 ] || { echo 'Remote recovery command exceeds the reviewed argument-size bound.' >&2; exit 66; }
rtk proxy ssh -o BatchMode=yes -o ConnectTimeout=8 nithin@10.100.1.10 "$REMOTE_COMMAND"
