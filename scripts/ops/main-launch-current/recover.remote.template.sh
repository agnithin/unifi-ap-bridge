#!/bin/sh
set -eu
HOST_EXPECTED='ZenWiFi_XT8-7B10'
STAGE='/tmp/unifi-main-launch-1f22343206c80bda01d576dfdb813e4a911e4568-202610261245261754'
SCRIPT_SHA='5635a27505b03d72777ae3a9763f3867c567fc6beef65d2019248e964b33e9f2'
[ "$(hostname)" = "$HOST_EXPECTED" ] || { echo 'recovery handoff refused: wrong host' >&2; exit 70; }
[ -d "$STAGE" ] || { echo 'recovery handoff refused: launch evidence directory missing' >&2; exit 71; }
RECOVERY_SCRIPT="$STAGE/recover-attempt-2.sh"
for evidence in "$RECOVERY_SCRIPT" "$STAGE/recovery-attempt-2.log" "$STAGE/recovery-attempt-2-launcher.status"; do
  [ ! -e "$evidence" ] || { echo 'recovery handoff refused: second recovery evidence already exists; preserve it' >&2; exit 72; }
done
umask 077
printf '%s' @SCRIPT_BODY@ > "$RECOVERY_SCRIPT"
chmod 700 "$RECOVERY_SCRIPT"
actual=$(openssl dgst -sha256 "$RECOVERY_SCRIPT" | awk '{print $NF}')
[ "$actual" = "$SCRIPT_SHA" ] || { echo 'recovery script checksum mismatch; retained for diagnosis' >&2; exit 73; }
sh -n "$RECOVERY_SCRIPT" || { echo 'recovery script syntax failure; retained for diagnosis' >&2; exit 74; }
if /bin/sh "$RECOVERY_SCRIPT" > "$STAGE/recovery-attempt-2.log" 2>&1; then
  recovery_rc=0
else
  recovery_rc=$?
fi
printf 'recovery_launcher_exit=%s\nremote_script=%s/recover-attempt-2.sh\nscript_sha256=%s\nlogs_and_snapshots=retained\n' "$recovery_rc" "$STAGE" "$actual" > "$STAGE/recovery-attempt-2-launcher.status"
chmod 600 "$STAGE/recovery-attempt-2-launcher.status"
[ ! -f "$STAGE/recovery.status" ] || cat "$STAGE/recovery.status"
if [ "$recovery_rc" -ne 0 ]; then
  grep -E '^recovery=STOP reason=' "$STAGE/recovery-attempt-2.log" || true
fi
cat "$STAGE/recovery-attempt-2-launcher.status"
exit "$recovery_rc"
