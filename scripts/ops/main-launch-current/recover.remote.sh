#!/bin/sh
set -eu

HOST_EXPECTED='ZenWiFi_XT8-7B10'
INSTALL='/jffs/addons/unifi-bridge'
CONFIG="$INSTALL/config/config.json"
STATE="$INSTALL/state.json"
KEY="$INSTALL/.ssh/cluster_key"
STAGE='/tmp/unifi-main-launch-1f22343206c80bda01d576dfdb813e4a911e4568-20261026054615'
STATUS='/tmp/var/wwwext/unifi-bridge/status.js'
UPDATE_LOCK='/tmp/unifi-bridge-update.lock'
MUTATION_LOCK='/tmp/unifi-bridge-mutation.lock'
START_LOCK='/tmp/unifi-bridge-start.lock'
ASSETS='unifi-bridge.tar.gz unifi-bridge.tar.gz.sha256 install.sh start.sh apply_config.sh sync_nodes.sh unifi-bridge.sh update.sh watchdog.sh version.txt webui/unifi-bridge.asp'

fail() { printf 'recovery=STOP reason=%s stage=%s\n' "$1" "$STAGE"; exit 90; }
mode() { ls -ln "$1" | awk '{print $3 ":" $4 ":" substr($1,1,10)}'; }
perm() { ls -ln "$1" | awk '{print substr($1,1,10)}'; }
compare() {
  if cmp -s "$1" "$2"; then
    printf same
    return 0
  else
    result=$?
  fi
  case "$result" in 0) printf same;; 1) printf changed;; *) printf error;; esac
}
state_identity() {
  input=$1; output=$2
  : > "$output" || return 1
  for field in schema mac adopted mode authkey paused; do
    value=$(sed -n "s/.*\"$field\"[[:space:]]*:[[:space:]]*\\([^,}]*\\).*/\\1/p" "$input")
    [ -n "$value" ] || return 1
    printf '%s=%s\n' "$field" "$value" >> "$output" || return 1
  done
}
uint() {
  case "$1" in ''|*[!0-9]*) return 1;; esac
}
status_is_fresh() {
  uint "$1" && uint "$2" && uint "$3" || return 1
  [ "$2" -le "$1" ] || return 1
  age=$(( $1 - $2 ))
  [ "$age" -le "$3" ]
}
# Use epoch fields for operation ordering and freshness. The daemon's
# zone-less last_successful_inform string is evidence only; its clock basis is
# not compared with shell wall time. The process starts with zeroed exchange
# health, so its nonempty success field plus Active (GCM) proves an exchange.
# Equal-second status is ambiguous.
fresh_inform_after_restart() {
  uint "$1" && uint "$2" && uint "$3" && uint "$4" || return 1
  status_is_fresh "$3" "$1" "$4" && [ "$1" -gt "$2" ] &&
    [ -n "$7" ] && [ "$5" = "$6" ] && [ "$8" = "$9" ] &&
    [ "${10}" = 'Active (GCM)' ]
}

[ "$(hostname)" = "$HOST_EXPECTED" ] || fail wrong_host
[ -d "$STAGE" ] || fail retained_stage_missing
[ ! -e "$INSTALL/.update-transaction" ] || fail transaction_requires_separate_journal_recovery
for path in "$UPDATE_LOCK" "$MUTATION_LOCK" "$START_LOCK"; do
  [ ! -e "$path" ] || fail active_or_unreconciled_lock
 done

[ -f "$STAGE/prelaunch-addon.tar.gz" ] && [ -f "$STAGE/prelaunch-runtime.tar.gz" ] || fail recovery_snapshot_missing
addon_sha=$(openssl dgst -sha256 "$STAGE/prelaunch-addon.tar.gz" | awk '{print $NF}')
runtime_sha=$(openssl dgst -sha256 "$STAGE/prelaunch-runtime.tar.gz" | awk '{print $NF}')
[ "$addon_sha" = "$(cat "$STAGE/prelaunch-addon.sha256")" ] || fail addon_snapshot_checksum_mismatch
[ "$runtime_sha" = "$(cat "$STAGE/prelaunch-runtime.sha256")" ] || fail runtime_snapshot_checksum_mismatch

# Verify preservation before stopping the candidate or changing installed assets.
[ -f "$STAGE/preserve/config.json" ] && [ -f "$STAGE/preserve/cluster_key" ] &&
  [ -f "$STAGE/preserve/state.identity" ] || fail preservation_baseline_missing
config_before=$(compare "$STAGE/preserve/config.json" "$CONFIG")
key_before=$(compare "$STAGE/preserve/cluster_key" "$KEY")
[ "$config_before" = same ] || fail config_bytes_not_preserved
[ "$key_before" = same ] || fail ssh_key_bytes_not_preserved
config_mode_before=$(mode "$STAGE/preserve/config.json")
config_mode_now=$(mode "$CONFIG")
key_mode_before=$(mode "$STAGE/preserve/cluster_key")
key_mode_now=$(mode "$KEY")
[ -n "$config_mode_before" ] && [ "$config_mode_before" = "$config_mode_now" ] || fail config_owner_group_mode_not_preserved
[ -n "$key_mode_before" ] && [ "$key_mode_before" = "$key_mode_now" ] || fail ssh_key_owner_group_mode_not_preserved
if [ -f "$STATE" ] && state_identity "$STATE" "$STAGE/preserve/state.identity.before"; then
  state_before=$(compare "$STAGE/preserve/state.identity" "$STAGE/preserve/state.identity.before")
else
  state_before=error
fi
[ "$state_before" = same ] || fail adoption_state_identity_not_preserved
state_mode_before=$(mode "$STAGE/preserve/state.json")
state_mode_now=$(mode "$STATE")
[ -n "$state_mode_before" ] && [ "$state_mode_before" = "$state_mode_now" ] || fail state_owner_group_mode_not_preserved
[ "$(perm "$CONFIG")" = '-rw-------' ] && [ "$(perm "$STATE")" = '-rw-------' ] &&
  [ "$(perm "$KEY")" = '-rw-------' ] && [ "$(perm "$INSTALL/.ssh")" = 'drwx------' ] || fail protected_file_permissions_unexpected

RESTORE_DIR="$STAGE/restore-$$"
mkdir -m 700 "$RESTORE_DIR" "$RESTORE_DIR/addon" "$RESTORE_DIR/runtime"
tar -xzpf "$STAGE/prelaunch-addon.tar.gz" -C "$RESTORE_DIR/addon"
tar -xzpf "$STAGE/prelaunch-runtime.tar.gz" -C "$RESTORE_DIR/runtime"
printf '%s\n' $ASSETS | LC_ALL=C sort > "$RESTORE_DIR/addon.expected"
find "$RESTORE_DIR/addon" -type f | sed "s#^$RESTORE_DIR/addon/##" | LC_ALL=C sort > "$RESTORE_DIR/addon.actual"
cmp -s "$RESTORE_DIR/addon.expected" "$RESTORE_DIR/addon.actual" || fail addon_snapshot_member_mismatch
[ -f "$RESTORE_DIR/runtime/unifi-bridge" ] || fail runtime_snapshot_member_missing
verify_hash() { [ "$(openssl dgst -sha256 "$1" | awk '{print $NF}')" = "$2" ]; }
verify_hash "$RESTORE_DIR/addon/unifi-bridge.tar.gz" '8199dc2446929de29f4a56e8acb4338626724a384a57fc33b6c86e1ef6d9d0bc' || fail prior_archive_snapshot_mismatch
verify_hash "$RESTORE_DIR/addon/unifi-bridge.tar.gz.sha256" '8f99e5e9a5cca107cad0a1ea68d555e25ac5d4561ac69baf76be9c0e099ac47d' || fail prior_archive_manifest_snapshot_mismatch
verify_hash "$RESTORE_DIR/addon/install.sh" '947b47aa210e87fa232d2d4363299a95098cc62979e7733a6cde49196c15f727' || fail prior_install_snapshot_mismatch
verify_hash "$RESTORE_DIR/addon/start.sh" 'c4f90d8d40d9641a558186aee034afa7313fd9e31ebc57c49027f123b4080455' || fail prior_start_snapshot_mismatch
verify_hash "$RESTORE_DIR/addon/apply_config.sh" 'ae49fa5ca31a45df39283d35b80ef8903092a32cd0a85924410c77ce6e3a1e3d' || fail prior_apply_config_snapshot_mismatch
verify_hash "$RESTORE_DIR/addon/sync_nodes.sh" 'a18ea1a7c30237592725efa876eca4859eb17cf8f881da7a95d6c51228f6c5e5' || fail prior_sync_snapshot_mismatch
verify_hash "$RESTORE_DIR/addon/unifi-bridge.sh" '9f69d3799f7aaf9261334fc1190fcdc6935f00a61e7baa983dc8edb04f723e35' || fail prior_bridge_wrapper_snapshot_mismatch
verify_hash "$RESTORE_DIR/addon/update.sh" 'fa51fbbe1383b67f0c4bcca002727d0ce558d9f2208a5f33f6622867b95b83e6' || fail prior_updater_snapshot_mismatch
verify_hash "$RESTORE_DIR/addon/watchdog.sh" '02c6fef5700164003825656dbcdea8cf7f3593a24a282e0b3fe20ab465057fd8' || fail prior_watchdog_snapshot_mismatch
verify_hash "$RESTORE_DIR/addon/version.txt" 'fe2b7c237b5a6e42818aa3e4f7fc5820f49c32289c32c96260ef89a8c8f6d3d7' || fail prior_version_snapshot_mismatch
verify_hash "$RESTORE_DIR/addon/webui/unifi-bridge.asp" '29c53d4a0734783ff6ac2e7d3ce759364a6817552686887a58031ff929d3be56' || fail prior_webui_snapshot_mismatch
verify_hash "$RESTORE_DIR/runtime/unifi-bridge" 'f213ec89609cdefbe3a445967247da2d9e25867de2e5394fdbb993e2df4a00ab' || fail prior_runtime_snapshot_mismatch

# Acquire the same lock order as the updater. Never remove or adopt an existing lock.
umask 077
update_owned=0
mutation_owned=0
start_owned=0
release_locks() {
  if [ "$start_owned" -eq 1 ] && [ "$(cat "$START_LOCK/pid" 2>/dev/null || true)" = "$$" ]; then rm -f "$START_LOCK/pid"; rmdir "$START_LOCK"; start_owned=0; fi
  if [ "$mutation_owned" -eq 1 ] && [ "$(cat "$MUTATION_LOCK/pid" 2>/dev/null || true)" = "$$" ]; then rm -f "$MUTATION_LOCK/pid"; rmdir "$MUTATION_LOCK"; mutation_owned=0; fi
  if [ "$update_owned" -eq 1 ] && [ "$(cat "$UPDATE_LOCK/pid" 2>/dev/null || true)" = "$$" ]; then rm -f "$UPDATE_LOCK/pid"; rmdir "$UPDATE_LOCK"; update_owned=0; fi
}
trap 'release_locks' 0
trap 'exit 130' INT
trap 'exit 143' TERM
mkdir -m 700 "$UPDATE_LOCK" || fail update_lock_busy
update_owned=1
if ! printf '%s\n' "$$" > "$UPDATE_LOCK/pid"; then rm -f "$UPDATE_LOCK/pid"; rmdir "$UPDATE_LOCK" 2>/dev/null || true; update_owned=0; fail update_lock_owner_write_failed; fi
mkdir -m 700 "$MUTATION_LOCK" || fail mutation_lock_busy
mutation_owned=1
if ! printf '%s\n' "$$" > "$MUTATION_LOCK/pid"; then rm -f "$MUTATION_LOCK/pid"; rmdir "$MUTATION_LOCK" 2>/dev/null || true; mutation_owned=0; fail mutation_lock_owner_write_failed; fi
mkdir -m 700 "$START_LOCK" || fail start_lock_busy
start_owned=1
if ! printf '%s\n' "$$" > "$START_LOCK/pid"; then rm -f "$START_LOCK/pid"; rmdir "$START_LOCK" 2>/dev/null || true; start_owned=0; fail start_lock_owner_write_failed; fi

# Do not mutate if a transaction appeared after the initial preflight.
[ ! -e "$INSTALL/.update-transaction" ] || fail transaction_requires_separate_journal_recovery

# Recheck protected paths while holding all updater locks, before stopping the daemon.
config_before=$(compare "$STAGE/preserve/config.json" "$CONFIG")
key_before=$(compare "$STAGE/preserve/cluster_key" "$KEY")
[ "$config_before" = same ] || fail config_bytes_changed_during_recovery_preflight
[ "$key_before" = same ] || fail ssh_key_bytes_changed_during_recovery_preflight
config_mode_now=$(mode "$CONFIG")
key_mode_now=$(mode "$KEY")
[ "$config_mode_before" = "$config_mode_now" ] && [ "$key_mode_before" = "$key_mode_now" ] || fail protected_file_modes_changed_during_recovery_preflight
if [ -f "$STATE" ] && state_identity "$STATE" "$STAGE/preserve/state.identity.locked"; then
  state_before=$(compare "$STAGE/preserve/state.identity" "$STAGE/preserve/state.identity.locked")
else
  state_before=error
fi
[ "$state_before" = same ] || fail state_identity_changed_during_recovery_preflight
state_mode_now=$(mode "$STATE")
[ "$state_mode_before" = "$state_mode_now" ] || fail state_mode_changed_during_recovery_preflight
[ "$(perm "$CONFIG")" = '-rw-------' ] && [ "$(perm "$STATE")" = '-rw-------' ] &&
  [ "$(perm "$KEY")" = '-rw-------' ] && [ "$(perm "$INSTALL/.ssh")" = 'drwx------' ] || fail protected_file_permissions_changed_during_recovery_preflight

pid_list=$(pidof unifi-bridge 2>/dev/null | tr -s '[:space:]' ' ' | sed 's/^ //;s/ $//')
if [ -n "$pid_list" ]; then
  case "$pid_list" in *[!0-9\ ]*) fail invalid_daemon_pid_list;; esac
  set -- $pid_list
  [ "$#" -eq 1 ] || fail multiple_daemon_pids
  kill "$1"
  tries=0
  while [ "$tries" -lt 15 ] && [ -n "$(pidof unifi-bridge 2>/dev/null | tr -d ' \t\r\n')" ]; do
    sleep 1
    tries=$((tries + 1))
  done
  [ -z "$(pidof unifi-bridge 2>/dev/null | tr -d ' \t\r\n')" ] || fail daemon_would_not_stop
fi

for rel in $ASSETS; do
  src="$RESTORE_DIR/addon/$rel"
  dst="$INSTALL/$rel"
  [ -f "$src" ] || fail restore_asset_missing
  mkdir -p "$(dirname "$dst")"
  tmp="$dst.restore.$$"
  cp -p "$src" "$tmp"
  mv -f "$tmp" "$dst"
done
runtime_tmp='/tmp/unifi-bridge/unifi-bridge.restore.'"$$"
cp -p "$RESTORE_DIR/runtime/unifi-bridge" "$runtime_tmp"
mv -f "$runtime_tmp" /tmp/unifi-bridge/unifi-bridge

for rel in $ASSETS; do
  cmp -s "$RESTORE_DIR/addon/$rel" "$INSTALL/$rel" || fail restored_asset_content_mismatch
  [ "$(mode "$RESTORE_DIR/addon/$rel")" = "$(mode "$INSTALL/$rel")" ] || fail restored_asset_mode_mismatch
done
[ "$(mode "$RESTORE_DIR/runtime/unifi-bridge")" = "$(mode /tmp/unifi-bridge/unifi-bridge)" ] || fail restored_runtime_mode_mismatch
cmp -s "$RESTORE_DIR/runtime/unifi-bridge" /tmp/unifi-bridge/unifi-bridge || fail restored_runtime_content_mismatch

release_locks
recovery_started=$(date +%s)
before_inform=$(sed -n 's/.*"last_successful_inform":[[:space:]]*"\([^"]*\)".*/\1/p' "$STATUS" | head -n 1)
printf '%s\n' "$before_inform" > "$STAGE/preserve/recovery.before_inform"
"$INSTALL/start.sh"
tries=0
while [ "$tries" -lt 25 ]; do
  current_inform=$(sed -n 's/.*"last_successful_inform":[[:space:]]*"\([^"]*\)".*/\1/p' "$STATUS" | head -n 1)
  updated=$(sed -n 's/.*"status_updated_at":[[:space:]]*\([0-9][0-9]*\).*/\1/p' "$STATUS" | head -n 1)
  live_pid_list=$(pidof unifi-bridge 2>/dev/null | tr -s '[:space:]' ' ' | sed 's/^ //;s/ $//')
  live_pid=''
  case "$live_pid_list" in *[!0-9\ ]*|'') ;; *) set -- $live_pid_list; [ "$#" -eq 1 ] && live_pid=$1;; esac
  status_pid=$(sed -n 's/.*"pid":[[:space:]]*\([0-9][0-9]*\).*/\1/p' "$STATUS" | head -n 1)
  status_commit=$(sed -n 's/.*"commit":[[:space:]]*"\([^"]*\)".*/\1/p' "$STATUS" | head -n 1)
  last_status=$(sed -n 's/.*"last_status":[[:space:]]*"\([^"]*\)".*/\1/p' "$STATUS" | head -n 1)
  observed_now=$(date +%s)
  if fresh_inform_after_restart "$updated" "$recovery_started" "$observed_now" 45 "$live_pid" "$status_pid" "$current_inform" "$status_commit" 'be106ec33fe6790af30a600d11bd05246445c530' "$last_status" && [ -n "$live_pid" ] && grep -Fq '"running": true' "$STATUS" && grep -Fq '"enabled": true' "$STATUS" && grep -Fq '"adopted": true' "$STATUS" && grep -Fq '"mode": "GCM"' "$STATUS"; then
    break
  fi
  sleep 2
  tries=$((tries + 1))
done
[ "$tries" -lt 25 ] || fail prior_runtime_inform_not_verified
runtime_identity=$(/tmp/unifi-bridge/unifi-bridge -version 2>/dev/null) || fail prior_runtime_identity_unavailable
printf '%s\n' "$runtime_identity" | grep -Fq 'unifi-bridge 1.2.5' || fail prior_runtime_version_mismatch
printf '%s\n' "$runtime_identity" | grep -Fq 'commit: be106ec33fe6790af30a600d11bd05246445c530' || fail prior_runtime_identity_mismatch
type cru >/dev/null 2>&1 && cru l 2>/dev/null | grep -Fq 'unifi_bridge_watchdog' && cru l 2>/dev/null | grep -Fq "$INSTALL/watchdog.sh" || fail watchdog_cron_missing
for path in "$INSTALL/.update-transaction" "$UPDATE_LOCK" "$MUTATION_LOCK" "$START_LOCK"; do
  [ ! -e "$path" ] || fail transaction_or_lock_remaining
done

config_result=$(compare "$STAGE/preserve/config.json" "$CONFIG")
key_result=$(compare "$STAGE/preserve/cluster_key" "$KEY")
if [ -f "$STATE" ] && state_identity "$STATE" "$STAGE/preserve/state.identity.recovery"; then
  state_result=$(compare "$STAGE/preserve/state.identity" "$STAGE/preserve/state.identity.recovery")
else
  state_result=error
fi
state_bytes_result=$(compare "$STAGE/preserve/state.json" "$STATE")
config_mode_result=error
state_mode_result=error
key_mode_result=error
[ "$(mode "$STAGE/preserve/config.json")" = "$(mode "$CONFIG")" ] && config_mode_result=same
[ "$(mode "$STAGE/preserve/state.json")" = "$(mode "$STATE")" ] && state_mode_result=same
[ "$(mode "$STAGE/preserve/cluster_key")" = "$(mode "$KEY")" ] && key_mode_result=same
printf 'recovery=restored_prior_assets\nprior_commit=be106ec33fe6790af30a600d11bd05246445c530\nconfig_bytes=%s\nconfig_owner_group_mode=%s\nstate_identity=%s\nstate_bytes=%s\nstate_owner_group_mode=%s\nssh_key_bytes=%s\nssh_key_owner_group_mode=%s\nlogs_and_snapshots=retained\n' "$config_result" "$config_mode_result" "$state_result" "$state_bytes_result" "$state_mode_result" "$key_result" "$key_mode_result" > "$STAGE/recovery.status"
chmod 600 "$STAGE/recovery.status"
cat "$STAGE/recovery.status"
[ "$config_result" = same ] && [ "$config_mode_result" = same ] && [ "$state_result" = same ] && [ "$state_mode_result" = same ] && [ "$key_result" = same ] && [ "$key_mode_result" = same ]
