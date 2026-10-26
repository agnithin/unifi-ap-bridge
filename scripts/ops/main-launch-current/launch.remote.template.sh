#!/bin/sh
set -eu

HOST_EXPECTED='ZenWiFi_XT8-7B10'
COMMIT='1f22343206c80bda01d576dfdb813e4a911e4568'
SCRIPT_SHA='2f2dc19723cf442a2ff5ea7ebd9e8b10b56b6a0c24ab2441eca602c14407547a'
REQUEST_ID='20261026070142'
INSTALL='/jffs/addons/unifi-bridge'
CONFIG="$INSTALL/config/config.json"
STATE_EXPECTED="$INSTALL/state.json"
KEY="$INSTALL/.ssh/cluster_key"
STATUS='/tmp/var/wwwext/unifi-bridge/status.js'
STAGE="/tmp/unifi-main-launch-${COMMIT}-20261026070142"

fail() { printf 'preflight=FAIL reason=%s stage=%s\n' "$1" "$STAGE"; exit 70; }
mode() { ls -ln "$1" | awk '{print $3 ":" $4 ":" substr($1,1,10)}'; }
perm() { ls -lnd "$1" | awk '{print substr($1,1,10)}'; }
file_state() {
  [ -e "$1" ] || { printf missing; return 0; }
  [ -f "$1" ] || { printf not_regular; return 0; }
  file_readable "$1" || { printf unreadable; return 0; }
  printf present
}
file_readable() { [ -r "$1" ]; }
compare_candidate_file() {
  source_state=$(file_state "$1")
  case "$source_state" in
    missing) printf source_missing; return 0;;
    unreadable) printf source_unreadable; return 0;;
    not_regular) printf source_not_regular; return 0;;
  esac
  destination_state=$(file_state "$2")
  case "$destination_state" in
    missing) printf destination_missing; return 0;;
    unreadable) printf destination_unreadable; return 0;;
    not_regular) printf destination_not_regular; return 0;;
  esac
  if cmp -s "$1" "$2"; then
    printf same
  else
    compare_rc=$?
    case "$compare_rc" in 1) printf different;; *) printf comparison_error;; esac
  fi
}
compare_candidate_mode() {
  source_mode=$(perm "$1" 2>/dev/null) || source_mode=
  [ -n "$source_mode" ] || { printf source_mode_error; return 0; }
  destination_mode=$(perm "$2" 2>/dev/null) || destination_mode=
  [ -n "$destination_mode" ] || { printf destination_mode_error; return 0; }
  [ "$source_mode" = "$destination_mode" ] && printf same || printf different
}
candidate_milestone() {
  milestone_key=$1; milestone_value=$2
  sed "s/^${milestone_key}=.*/${milestone_key}=${milestone_value}/" "$STAGE/candidate-milestones.status" > "$STAGE/candidate-milestones.tmp" &&
    mv -f "$STAGE/candidate-milestones.tmp" "$STAGE/candidate-milestones.status"
}
record_candidate_file() {
  candidate_rel=$1; candidate_source=$2; candidate_destination=$3; candidate_check_mode=$4
  candidate_content=$(compare_candidate_file "$candidate_source" "$candidate_destination")
  if [ "$candidate_check_mode" = compare ]; then
    candidate_mode=$(compare_candidate_mode "$candidate_source" "$candidate_destination")
  else
    candidate_mode=$(perm "$candidate_destination" 2>/dev/null) || candidate_mode=
    if [ -z "$candidate_mode" ]; then
      candidate_mode=destination_mode_error
    elif [ "$candidate_mode" = "$candidate_check_mode" ]; then
      candidate_mode=same
    else
      candidate_mode=different
    fi
  fi
  printf '%s content=%s mode=%s\n' "$candidate_rel" "$candidate_content" "$candidate_mode" >> "$STAGE/candidate-files.status"
  [ "$candidate_content" = same ] && [ "$candidate_mode" = same ]
}
state_identity() {
  input=$1; output=$2
  : > "$output" || return 1
  for field in schema mac adopted mode authkey paused; do
    value=$(sed -n "s/.*\"$field\"[[:space:]]*:[[:space:]]*\\([^,}]*\\).*/\\1/p" "$input")
    if [ -z "$value" ]; then
      if [ "$field" = paused ] && ! grep -Fq '"paused"' "$input"; then
        value=false
      else
        return 1
      fi
    fi
    printf '%s=%s\n' "$field" "$value" >> "$output" || return 1
  done
}
watchdog_present() {
  type cru >/dev/null 2>&1 && cru l 2>/dev/null | grep -Fq 'unifi_bridge_watchdog' &&
    cru l 2>/dev/null | grep -Fq "$INSTALL/watchdog.sh"
}
locks_absent() {
  for lock_path do
    [ ! -e "$lock_path" ] || return 1
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
# The zone-less inform string is deliberately opaque: the Go process may format
# it in a different timezone from the shell. A post-restart epoch status, the
# sole live PID, expected commit, and Active (GCM) with a nonempty inform prove
# the new process completed an exchange (exchangeHealth starts zeroed per
# process); equal-second status is ambiguous.
fresh_inform_after_restart() {
  uint "$1" && uint "$2" && uint "$3" && uint "$4" || return 1
  status_is_fresh "$3" "$1" "$4" && [ "$1" -gt "$2" ] &&
    [ -n "$7" ] && [ "$5" = "$6" ] && [ "$8" = "$9" ] &&
    [ "${10}" = 'Active (GCM)' ]
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
status_pid=$(sed -n 's/.*"pid":[[:space:]]*\([0-9][0-9]*\).*/\1/p' "$STATUS" | head -n 1)
status_is_fresh "$now" "$status_updated" 45 || fail stale_status
[ "$status_pid" = "$pid" ] || fail stale_status_pid_mismatch
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
printf 'updater_exit=%s\narchive=not_checked\narchive_checksum=not_checked\narchive_member_list=not_checked\narchive_extraction=not_checked\narchive_members=not_checked\nbundle_manifest=not_checked\nfile_comparisons=not_checked\n' "$updater_rc" > "$STAGE/candidate-milestones.status"
printf 'file content=code mode=code\n' > "$STAGE/candidate-files.status"
chmod 600 "$STAGE/candidate-milestones.status" "$STAGE/candidate-files.status"
if [ "$updater_rc" -eq 0 ]; then
  if [ ! -f "$INSTALL/unifi-bridge.tar.gz" ]; then
    candidate_milestone archive missing
  else
    candidate_milestone archive present
    archive_sha=$(openssl dgst -sha256 "$INSTALL/unifi-bridge.tar.gz" 2>/dev/null | awk '{print $NF}')
    if [ "$archive_sha" != '16300c4bc0f08f6d5bfadae8427a7928f01de1470eaf7d000c317be6d27657f1' ]; then
      candidate_milestone archive_checksum mismatch_or_error
    else
      candidate_milestone archive_checksum pass
      if ! mkdir -m 700 "$STAGE/candidate-check"; then
        candidate_milestone archive_extraction destination_error
      elif ! tar -tzf "$INSTALL/unifi-bridge.tar.gz" > "$STAGE/candidate.members"; then
        candidate_milestone archive_member_list error
      else
        candidate_milestone archive_member_list pass
        if ! tar -xzpf "$INSTALL/unifi-bridge.tar.gz" -C "$STAGE/candidate-check"; then
          candidate_milestone archive_extraction error
        else
          candidate_milestone archive_extraction pass
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
             LC_ALL=C sort "$STAGE/candidate.members" > "$STAGE/candidate.actual-members"; then
            if cmp -s "$STAGE/candidate.expected-members" "$STAGE/candidate.actual-members"; then
              candidate_milestone archive_members pass
            else
              member_rc=$?
              if [ "$member_rc" -eq 1 ]; then candidate_milestone archive_members different
              else candidate_milestone archive_members comparison_error; fi
            fi
            if [ "$(sed -n 's/^archive_members=//p' "$STAGE/candidate-milestones.status")" != pass ]; then
              candidate_milestone bundle_manifest not_checked_members
            else
              if grep -Fq '"commit": "1f22343206c80bda01d576dfdb813e4a911e4568"' "$STAGE/candidate-check/bundle.json" &&
                 grep -Fq '"architecture": "armv7"' "$STAGE/candidate-check/bundle.json" &&
                 [ "$(awk 'NF==2 && $2=="unifi-bridge.tar.gz" {print $1}' "$INSTALL/unifi-bridge.tar.gz.sha256")" = '16300c4bc0f08f6d5bfadae8427a7928f01de1470eaf7d000c317be6d27657f1' ]; then
                candidate_milestone bundle_manifest pass
                candidate_bundle=pass
                for rel in install.sh start.sh apply_config.sh sync_nodes.sh unifi-bridge.sh update.sh watchdog.sh; do
                  record_candidate_file "$rel" "$STAGE/candidate-check/$rel" "$INSTALL/$rel" compare || candidate_bundle=fail
                done
                record_candidate_file webui/unifi-bridge.asp "$STAGE/candidate-check/unifi-bridge.asp" "$INSTALL/webui/unifi-bridge.asp" '-rw-r--r--' || candidate_bundle=fail
                record_candidate_file version.txt "$STAGE/candidate-check/version.txt" "$INSTALL/version.txt" '-rw-r--r--' || candidate_bundle=fail
                record_candidate_file unifi-bridge-armv7 "$STAGE/candidate-check/unifi-bridge-armv7" /tmp/unifi-bridge/unifi-bridge '-rwxr-xr-x' || candidate_bundle=fail
                if [ "$candidate_bundle" = pass ]; then candidate_milestone file_comparisons pass
                else candidate_milestone file_comparisons fail; fi
              else
                candidate_milestone bundle_manifest invalid_or_missing
              fi
            fi
          else
            candidate_milestone archive_members listing_error
          fi
        fi
      fi
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
locks_result=absent
locks_absent /tmp/unifi-bridge-update.lock /tmp/unifi-bridge-mutation.lock /tmp/unifi-bridge-start.lock || locks_result=present
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
  candidate_status_commit=$(printf '%s\n' "$candidate_status" | sed -n 's/.*"commit":[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)
  candidate_last_status=$(printf '%s\n' "$candidate_status" | sed -n 's/.*"last_status":[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)
  candidate_observed_now=$(date +%s)
  candidate_identity=$(/tmp/unifi-bridge/unifi-bridge -version 2>/dev/null || true)
  if fresh_inform_after_restart "$candidate_updated" "$updater_started" "$candidate_observed_now" 45 "$candidate_pid" "$candidate_status_pid" "$candidate_inform" "$candidate_status_commit" "$COMMIT" "$candidate_last_status" &&
     printf '%s\n' "$candidate_identity" | grep -Fq "commit: $COMMIT" &&
     printf '%s\n' "$candidate_identity" | grep -Fq 'unifi-bridge 1.2.5' &&
     printf '%s\n' "$candidate_status" | grep -Fq '"adopted": true' &&
     printf '%s\n' "$candidate_status" | grep -Fq '"mode": "GCM"' &&
     printf '%s\n' "$candidate_status" | grep -Fq '"running": true'; then
    candidate_runtime=pass
  fi
fi
watchdog_result=absent
watchdog_present && watchdog_result=present
printf 'updater_exit=%s\ncandidate_bundle=%s\ncandidate_runtime=%s\nwatchdog=%s\nconfig_bytes=%s\nconfig_owner_group_mode=%s\nstate_identity=%s\nstate_bytes=%s\nstate_owner_group_mode=%s\nssh_key_bytes=%s\nssh_key_owner_group_mode=%s\ntransaction=%s\nlocks=%s\n' \
  "$updater_rc" "$candidate_bundle" "$candidate_runtime" "$watchdog_result" "$config_result" "$config_mode_result" "$state_result" "$state_bytes_result" "$state_mode_result" "$key_result" "$key_mode_result" "$journal_result" "$locks_result" > "$STAGE/worker.status"
chmod 600 "$STAGE/worker.status"
chmod 600 "$STAGE/candidate-milestones.status" "$STAGE/candidate-files.status"
grep -E '"(version|commit|pid|adopted|mode|last_successful_inform|last_status)"' "$STATUS" > "$STAGE/postflight.safe" 2>/dev/null || true
printf 'stage=%s\n' "$STAGE"
cat "$STAGE/worker.status"
printf 'logs_and_snapshots=retained\n'
printf 'candidate_diagnostics=retained\n'
[ "$updater_rc" -eq 0 ] || exit "$updater_rc"
[ "$candidate_bundle" = pass ] || exit 87
[ "$candidate_runtime" = pass ] || exit 85
[ "$watchdog_result" = present ] || exit 86
[ "$config_result" = same ] && [ "$config_mode_result" = same ] || exit 81
[ "$state_result" = same ] && [ "$state_mode_result" = same ] || exit 82
[ "$key_result" = same ] && [ "$key_mode_result" = same ] || exit 83
[ "$journal_result" = absent ] || exit 84
[ "$locks_result" = absent ] || exit 88
exit 0
