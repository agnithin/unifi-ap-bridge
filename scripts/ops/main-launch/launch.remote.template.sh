#!/bin/sh
set -eu

HOST_EXPECTED='ZenWiFi_XT8-7B10'
COMMIT='a25bb6b272e6dcbe938b61fbca8d17044c6ff07d'
SCRIPT_SHA='2f2dc19723cf442a2ff5ea7ebd9e8b10b56b6a0c24ab2441eca602c14407547a'
REQUEST_ID='20261026011541'
INSTALL='/jffs/addons/unifi-bridge'
CONFIG="$INSTALL/config/config.json"
STATE_EXPECTED="$INSTALL/state.json"
KEY="$INSTALL/.ssh/cluster_key"
STATUS='/tmp/var/wwwext/unifi-bridge/status.js'
STAGE="/tmp/unifi-main-launch-${COMMIT}-20261026011541"

fail() { printf 'preflight=FAIL reason=%s stage=%s\n' "$1" "$STAGE"; exit 70; }
mode() { ls -ln "$1" | awk '{print $3 ":" $4 ":" substr($1,1,10)}'; }
perm() { ls -ln "$1" | awk '{print substr($1,1,10)}'; }
state_identity() {
  input=$1; output=$2
  : > "$output" || return 1
  for field in schema mac adopted mode authkey paused; do
    value=$(sed -n "s/.*\"$field\"[[:space:]]*:[[:space:]]*\\([^,}]*\\).*/\\1/p" "$input")
    [ -n "$value" ] || return 1
    printf '%s=%s\n' "$field" "$value" >> "$output" || return 1
  done
}
watchdog_present() {
  type cru >/dev/null 2>&1 && cru l 2>/dev/null | grep -Fq 'unifi_bridge_watchdog' &&
    cru l 2>/dev/null | grep -Fq "$INSTALL/watchdog.sh"
}

[ "$(hostname)" = "$HOST_EXPECTED" ] || fail wrong_host
[ ! -e "$STAGE" ] || fail stage_already_exists
umask 077
mkdir -m 700 "$STAGE"
mkdir -m 700 "$STAGE/preserve"

pid_list=$(pidof unifi-bridge 2>/dev/null | tr -s '[:space:]' ' ' | sed 's/^ //;s/ $//')
[ -n "$pid_list" ] || fail daemon_not_running
case "$pid_list" in *[!0-9\ ]*) fail invalid_daemon_pid_list;; esac
set -- $pid_list
[ "$#" -eq 1 ] || fail multiple_daemon_pids
pid=$1
args=$(tr '\000' '\n' < "/proc/$pid/cmdline")
config_arg=$(printf '%s\n' "$args" | awk 'previous == "-config" {print; exit} {previous=$0}')
[ "$config_arg" = "$CONFIG" ] || fail daemon_config_arg_mismatch
if printf '%s\n' "$args" | grep -Eq '^-state(=|$)'; then fail daemon_state_override; fi
state_path=$(sed -n 's/.*"state_path"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$CONFIG" | head -n 1)
[ "$state_path" = "$STATE_EXPECTED" ] || fail effective_state_path_mismatch

now=$(date +%s)
status_updated=$(sed -n 's/.*"status_updated_at":[[:space:]]*\([0-9][0-9]*\).*/\1/p' "$STATUS" | head -n 1)
[ -n "$status_updated" ] || fail missing_status_timestamp
age=$((now - status_updated))
[ "$age" -ge 0 ] && [ "$age" -le 45 ] || fail stale_status
for expected in '"commit": "be106ec33fe6790af30a600d11bd05246445c530"' '"adopted": true' '"mode": "GCM"' '"last_status": "Active (GCM)"'; do
  grep -Fq "$expected" "$STATUS" || fail prior_daemon_not_healthy
done
inform=$(sed -n 's/.*"last_successful_inform":[[:space:]]*"\([^"]*\)".*/\1/p' "$STATUS" | head -n 1)
[ -n "$inform" ] || fail missing_successful_inform
watchdog_present || fail watchdog_cron_missing
runtime_identity=$(/tmp/unifi-bridge/unifi-bridge -version 2>/dev/null) || fail prior_runtime_unavailable
printf '%s\n' "$runtime_identity" | grep -Fq 'commit: be106ec33fe6790af30a600d11bd05246445c530' || fail prior_runtime_identity_mismatch

for path in "$INSTALL/.update-transaction" /tmp/unifi-bridge-update.lock /tmp/unifi-bridge-mutation.lock /tmp/unifi-bridge-start.lock; do
  [ ! -e "$path" ] || fail preexisting_transaction_or_lock
done
[ "$(openssl dgst -sha256 "$INSTALL/unifi-bridge.tar.gz" | awk '{print $NF}')" = '8199dc2446929de29f4a56e8acb4338626724a384a57fc33b6c86e1ef6d9d0bc' ] || fail prior_archive_changed
verify_hash() { [ "$(openssl dgst -sha256 "$1" | awk '{print $NF}')" = "$2" ]; }
verify_hash "$INSTALL/unifi-bridge.tar.gz.sha256" '8f99e5e9a5cca107cad0a1ea68d555e25ac5d4561ac69baf76be9c0e099ac47d' || fail prior_archive_manifest_changed
verify_hash "$INSTALL/install.sh" '947b47aa210e87fa232d2d4363299a95098cc62979e7733a6cde49196c15f727' || fail prior_install_script_changed
verify_hash "$INSTALL/start.sh" 'c4f90d8d40d9641a558186aee034afa7313fd9e31ebc57c49027f123b4080455' || fail prior_start_script_changed
verify_hash "$INSTALL/apply_config.sh" 'ae49fa5ca31a45df39283d35b80ef8903092a32cd0a85924410c77ce6e3a1e3d' || fail prior_apply_config_changed
verify_hash "$INSTALL/sync_nodes.sh" 'a18ea1a7c30237592725efa876eca4859eb17cf8f881da7a95d6c51228f6c5e5' || fail prior_sync_script_changed
verify_hash "$INSTALL/unifi-bridge.sh" '9f69d3799f7aaf9261334fc1190fcdc6935f00a61e7baa983dc8edb04f723e35' || fail prior_bridge_wrapper_changed
verify_hash "$INSTALL/update.sh" 'fa51fbbe1383b67f0c4bcca002727d0ce558d9f2208a5f33f6622867b95b83e6' || fail prior_updater_changed
verify_hash "$INSTALL/watchdog.sh" '02c6fef5700164003825656dbcdea8cf7f3593a24a282e0b3fe20ab465057fd8' || fail prior_watchdog_changed
verify_hash "$INSTALL/version.txt" 'fe2b7c237b5a6e42818aa3e4f7fc5820f49c32289c32c96260ef89a8c8f6d3d7' || fail prior_version_changed
verify_hash "$INSTALL/webui/unifi-bridge.asp" '29c53d4a0734783ff6ac2e7d3ce759364a6817552686887a58031ff929d3be56' || fail prior_webui_changed
verify_hash /tmp/unifi-bridge/unifi-bridge 'f213ec89609cdefbe3a445967247da2d9e25867de2e5394fdbb993e2df4a00ab' || fail prior_runtime_changed
[ -f "$CONFIG" ] && [ -f "$STATE_EXPECTED" ] && [ -f "$KEY" ] || fail protected_file_missing
[ "$(perm "$CONFIG")" = '-rw-------' ] && [ "$(perm "$STATE_EXPECTED")" = '-rw-------' ] &&
  [ "$(perm "$KEY")" = '-rw-------' ] && [ "$(perm "$INSTALL/.ssh")" = 'drwx------' ] || fail protected_file_permissions_unexpected

# Private same-device rollback baseline; do not print or hash protected contents.
cp -p "$CONFIG" "$STAGE/preserve/config.json"
cp -p "$STATE_EXPECTED" "$STAGE/preserve/state.json"
cp -p "$KEY" "$STAGE/preserve/cluster_key"
state_identity "$STATE_EXPECTED" "$STAGE/preserve/state.identity" || fail state_identity_baseline_invalid
chmod 600 "$STAGE/preserve/state.identity"
mode "$CONFIG" > "$STAGE/preserve/config.json.mode"
mode "$STATE_EXPECTED" > "$STAGE/preserve/state.json.mode"
mode "$KEY" > "$STAGE/preserve/cluster_key.mode"
printf '%s\n' "$inform" > "$STAGE/preserve/last_successful_inform"

assets='unifi-bridge.tar.gz unifi-bridge.tar.gz.sha256 install.sh start.sh apply_config.sh sync_nodes.sh unifi-bridge.sh update.sh watchdog.sh version.txt webui/unifi-bridge.asp'
tar -czpf "$STAGE/prelaunch-addon.tar.gz" -C "$INSTALL" $assets
tar -czpf "$STAGE/prelaunch-runtime.tar.gz" -C /tmp/unifi-bridge unifi-bridge
printf '%s\n' $assets | LC_ALL=C sort > "$STAGE/expected-addon.members"
tar -tzf "$STAGE/prelaunch-addon.tar.gz" | LC_ALL=C sort > "$STAGE/prelaunch-addon.members"
cmp -s "$STAGE/expected-addon.members" "$STAGE/prelaunch-addon.members" || fail recovery_asset_snapshot_incomplete
tar -tzf "$STAGE/prelaunch-runtime.tar.gz" | grep -Fxq 'unifi-bridge' || fail runtime_snapshot_incomplete
openssl dgst -sha256 "$STAGE/prelaunch-addon.tar.gz" | awk '{print $NF}' > "$STAGE/prelaunch-addon.sha256"
openssl dgst -sha256 "$STAGE/prelaunch-runtime.tar.gz" | awk '{print $NF}' > "$STAGE/prelaunch-runtime.sha256"

printf 'host=%s\npid=%s\nprior_commit=be106ec33fe6790af30a600d11bd05246445c530\nstate_path=%s\nlast_successful_inform=%s\n' "$HOST_EXPECTED" "$pid" "$state_path" "$inform" > "$STAGE/preflight.safe"
grep -E '"(version|commit|pid|adopted|mode|last_successful_inform|last_status)"' "$STATUS" >> "$STAGE/preflight.safe"

printf '%s' @SCRIPT_BODY@ > "$STAGE/update.sh"
chmod 700 "$STAGE/update.sh"
actual=$(openssl dgst -sha256 "$STAGE/update.sh" | awk '{print $NF}')
[ "$actual" = "$SCRIPT_SHA" ] || fail candidate_script_checksum_mismatch
sh -n "$STAGE/update.sh" || fail candidate_script_syntax
printf 'candidate_script_sha256=%s\nremote_script=%s/update.sh\nsyntax=ok\n' "$actual" "$STAGE" >> "$STAGE/preflight.safe"

# Foreground invocation: the verified absolute path is used in this same SSH session.
updater_started=$(date +%s)
if /bin/sh "$STAGE/update.sh" apply-exact dev "$COMMIT" "$REQUEST_ID" > "$STAGE/apply.log" 2>&1; then
  updater_rc=0
else
  updater_rc=$?
fi

candidate_bundle=fail
if [ "$updater_rc" -eq 0 ] && [ -f "$INSTALL/unifi-bridge.tar.gz" ] &&
   [ "$(openssl dgst -sha256 "$INSTALL/unifi-bridge.tar.gz" | awk '{print $NF}')" = 'cdd11384e69da61775b75fe43b4b9d48bba54b4984f39c516f757128e23422a5' ]; then
  if mkdir -m 700 "$STAGE/candidate-check" &&
     tar -tzf "$INSTALL/unifi-bridge.tar.gz" > "$STAGE/candidate.members" &&
     tar -xzpf "$INSTALL/unifi-bridge.tar.gz" -C "$STAGE/candidate-check"; then
    expected_members='apply_config.sh
bundle.json
install.sh
start.sh
sync_nodes.sh
unifi-bridge-armv7
unifi-bridge.asp
unifi-bridge.sh
update.sh
version.txt
watchdog.sh'
    if printf '%s\n' "$expected_members" | LC_ALL=C sort > "$STAGE/candidate.expected-members" &&
       LC_ALL=C sort "$STAGE/candidate.members" > "$STAGE/candidate.actual-members" &&
       cmp -s "$STAGE/candidate.expected-members" "$STAGE/candidate.actual-members" &&
       grep -Fq '"commit": "a25bb6b272e6dcbe938b61fbca8d17044c6ff07d"' "$STAGE/candidate-check/bundle.json" &&
       grep -Fq '"architecture": "armv7"' "$STAGE/candidate-check/bundle.json" &&
       [ "$(awk 'NF==2 && $2=="unifi-bridge.tar.gz" {print $1}' "$INSTALL/unifi-bridge.tar.gz.sha256")" = 'cdd11384e69da61775b75fe43b4b9d48bba54b4984f39c516f757128e23422a5' ]; then
      candidate_bundle=pass
      for rel in install.sh start.sh apply_config.sh sync_nodes.sh unifi-bridge.sh update.sh watchdog.sh; do
        if ! cmp -s "$STAGE/candidate-check/$rel" "$INSTALL/$rel" || [ "$(perm "$STAGE/candidate-check/$rel")" != "$(perm "$INSTALL/$rel")" ]; then candidate_bundle=fail; fi
      done
      if ! cmp -s "$STAGE/candidate-check/unifi-bridge.asp" "$INSTALL/webui/unifi-bridge.asp" ||
         ! cmp -s "$STAGE/candidate-check/version.txt" "$INSTALL/version.txt" ||
         ! cmp -s "$STAGE/candidate-check/unifi-bridge-armv7" /tmp/unifi-bridge/unifi-bridge ||
         [ "$(perm "$INSTALL/version.txt")" != '-rw-r--r--' ] ||
         [ "$(perm "$INSTALL/webui/unifi-bridge.asp")" != '-rw-r--r--' ] ||
         [ "$(perm /tmp/unifi-bridge/unifi-bridge)" != '-rwxr-xr-x' ]; then candidate_bundle=fail; fi
    fi
  fi
fi

compare() {
  if cmp -s "$1" "$2"; then
    printf same
    return 0
  else
    result=$?
  fi
  case "$result" in 0) printf same;; 1) printf changed;; *) printf error;; esac
}
permission_compare() {
  [ -e "$2" ] || { printf missing; return 0; }
  expected_mode=$(mode "$1") || { printf error; return 0; }
  actual_mode=$(mode "$2") || { printf error; return 0; }
  [ -n "$expected_mode" ] && [ -n "$actual_mode" ] || { printf error; return 0; }
  [ "$expected_mode" = "$actual_mode" ] && printf same || printf changed
}
config_result=$(compare "$STAGE/preserve/config.json" "$CONFIG")
key_result=$(compare "$STAGE/preserve/cluster_key" "$KEY")
if [ -f "$STATE_EXPECTED" ] && state_identity "$STATE_EXPECTED" "$STAGE/preserve/state.identity.after"; then
  state_result=$(compare "$STAGE/preserve/state.identity" "$STAGE/preserve/state.identity.after")
else
  state_result=error
fi
state_bytes_result=$(compare "$STAGE/preserve/state.json" "$STATE_EXPECTED")
config_mode_result=$(permission_compare "$STAGE/preserve/config.json" "$CONFIG")
state_mode_result=$(permission_compare "$STAGE/preserve/state.json" "$STATE_EXPECTED")
key_mode_result=$(permission_compare "$STAGE/preserve/cluster_key" "$KEY")
journal_result=absent
[ ! -e "$INSTALL/.update-transaction" ] || journal_result=present
candidate_runtime=fail
candidate_pid_list=$(pidof unifi-bridge 2>/dev/null | tr -s '[:space:]' ' ' | sed 's/^ //;s/ $//')
case "$candidate_pid_list" in *[!0-9\ ]*|'') candidate_pid_list=invalid;; esac
set -- $candidate_pid_list
if [ "$candidate_pid_list" != invalid ] && [ "$#" -eq 1 ]; then
  candidate_pid=$1
  candidate_status=$(cat "$STATUS" 2>/dev/null)
  candidate_updated=$(printf '%s\n' "$candidate_status" | sed -n 's/.*"status_updated_at":[[:space:]]*\([0-9][0-9]*\).*/\1/p' | head -n 1)
  candidate_status_pid=$(printf '%s\n' "$candidate_status" | sed -n 's/.*"pid":[[:space:]]*\([0-9][0-9]*\).*/\1/p' | head -n 1)
  candidate_inform=$(printf '%s\n' "$candidate_status" | sed -n 's/.*"last_successful_inform":[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)
  candidate_identity=$(/tmp/unifi-bridge/unifi-bridge -version 2>/dev/null || true)
  if [ -n "$candidate_updated" ] && [ "$candidate_updated" -ge "$updater_started" ] &&
     [ "$candidate_status_pid" = "$candidate_pid" ] && [ -n "$candidate_inform" ] && [ "$candidate_inform" != "$inform" ] &&
     printf '%s\n' "$candidate_identity" | grep -Fq "commit: $COMMIT" &&
     printf '%s\n' "$candidate_identity" | grep -Fq 'unifi-bridge 1.2.5' &&
     printf '%s\n' "$candidate_status" | grep -Fq '"commit": "a25bb6b272e6dcbe938b61fbca8d17044c6ff07d"' &&
     printf '%s\n' "$candidate_status" | grep -Fq '"adopted": true' &&
     printf '%s\n' "$candidate_status" | grep -Fq '"mode": "GCM"' &&
     printf '%s\n' "$candidate_status" | grep -Fq '"last_status": "Active (GCM)"' &&
     printf '%s\n' "$candidate_status" | grep -Fq '"running": true'; then
    candidate_runtime=pass
  fi
fi
watchdog_result=absent
watchdog_present && watchdog_result=present
printf 'updater_exit=%s\ncandidate_bundle=%s\ncandidate_runtime=%s\nwatchdog=%s\nconfig_bytes=%s\nconfig_owner_group_mode=%s\nstate_identity=%s\nstate_bytes=%s\nstate_owner_group_mode=%s\nssh_key_bytes=%s\nssh_key_owner_group_mode=%s\ntransaction=%s\n' \
  "$updater_rc" "$candidate_bundle" "$candidate_runtime" "$watchdog_result" "$config_result" "$config_mode_result" "$state_result" "$state_bytes_result" "$state_mode_result" "$key_result" "$key_mode_result" "$journal_result" > "$STAGE/worker.status"
chmod 600 "$STAGE/worker.status"
grep -E '"(version|commit|pid|adopted|mode|last_successful_inform|last_status)"' "$STATUS" > "$STAGE/postflight.safe" 2>/dev/null || true
printf 'stage=%s\n' "$STAGE"
cat "$STAGE/worker.status"
printf 'logs_and_snapshots=retained\n'
[ "$updater_rc" -eq 0 ] || exit "$updater_rc"
[ "$candidate_bundle" = pass ] || exit 87
[ "$candidate_runtime" = pass ] || exit 85
[ "$watchdog_result" = present ] || exit 86
[ "$config_result" = same ] && [ "$config_mode_result" = same ] || exit 81
[ "$state_result" = same ] && [ "$state_mode_result" = same ] || exit 82
[ "$key_result" = same ] && [ "$key_mode_result" = same ] || exit 83
[ "$journal_result" = absent ] || exit 84
exit 0
