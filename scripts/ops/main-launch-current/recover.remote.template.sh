#!/bin/sh
set -eu
HOST_EXPECTED='ZenWiFi_XT8-7B10'
STAGE='/tmp/unifi-main-launch-1f22343206c80bda01d576dfdb813e4a911e4568-20261026054615'
SCRIPT_SHA='f3b4c00e5ea578a2f0da8fe8144dab5000f694b247b7ac0a321876fd4572ebef'
[ "$(hostname)" = "$HOST_EXPECTED" ] || { echo 'recovery handoff refused: wrong host' >&2; exit 70; }
[ -d "$STAGE" ] || { echo 'recovery handoff refused: launch evidence directory missing' >&2; exit 71; }
[ ! -e "$STAGE/recover.sh" ] || { echo 'recovery handoff refused: prior recovery handoff file exists; preserve evidence' >&2; exit 72; }
umask 077
printf '%s' @SCRIPT_BODY@ > "$STAGE/recover.sh"
chmod 700 "$STAGE/recover.sh"
actual=$(openssl dgst -sha256 "$STAGE/recover.sh" | awk '{print $NF}')
[ "$actual" = "$SCRIPT_SHA" ] || { echo 'recovery script checksum mismatch; retained for diagnosis' >&2; exit 73; }
sh -n "$STAGE/recover.sh" || { echo 'recovery script syntax failure; retained for diagnosis' >&2; exit 74; }
if /bin/sh "$STAGE/recover.sh" > "$STAGE/recovery-apply.log" 2>&1; then
  recovery_rc=0
else
  recovery_rc=$?
fi
printf 'recovery_launcher_exit=%s\nremote_script=%s/recover.sh\nscript_sha256=%s\nlogs_and_snapshots=retained\n' "$recovery_rc" "$STAGE" "$actual" > "$STAGE/recovery-launcher.status"
chmod 600 "$STAGE/recovery-launcher.status"
[ ! -f "$STAGE/recovery.status" ] || cat "$STAGE/recovery.status"
cat "$STAGE/recovery-launcher.status"
exit "$recovery_rc"
