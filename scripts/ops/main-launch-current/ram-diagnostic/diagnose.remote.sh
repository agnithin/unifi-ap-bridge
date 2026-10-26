#!/bin/sh
# RAM-only update.sh disappearance diagnostic. This script is a packet payload;
# it is not the candidate updater and must only be run after separate approval.
set -eu

HOST_EXPECTED='ZenWiFi_XT8-7B10'
COMMIT='1f22343206c80bda01d576dfdb813e4a911e4568'
ARCHIVE_SHA='16300c4bc0f08f6d5bfadae8427a7928f01de1470eaf7d000c317be6d27657f1'
SIDECAR_SHA='113a1ffc283a5a27ba1594736d827088aacab9e7936478234e0c775677eaf48f'
UPDATER_SHA='2f2dc19723cf442a2ff5ea7ebd9e8b10b56b6a0c24ab2441eca602c14407547a'
BASE_COMMIT='be106ec33fe6790af30a600d11bd05246445c530'
STATUS='/tmp/var/wwwext/unifi-bridge/status.js'
STAGE=${1:?stage path required}
PAYLOAD="$STAGE/payload"
LOG="$STAGE/observations.log"

fail() { printf 'diagnostic=STOP reason=%s\n' "$1"; exit 70; }
sha256() { openssl dgst -sha256 "$1" 2>/dev/null | awk '{print $NF}'; }
status_field() {
  sed -n "s/.*\"$1\":[[:space:]]*\([0-9][0-9]*\).*/\1/p" "$STATUS" | head -n 1
}
fresh_status() {
  now=$(date +%s) || return 1
  updated=$(status_field status_updated_at) || return 1
  case "$now:$updated" in *[!0-9:]*) return 1;; esac
  [ -n "$updated" ] && [ "$updated" -le "$now" ] && [ $((now - updated)) -le 45 ]
}
baseline() {
  [ "$(hostname 2>/dev/null)" = "$HOST_EXPECTED" ] || return 1
  pids=$(pidof unifi-bridge 2>/dev/null | tr -s '[:space:]' ' ' | sed 's/^ //;s/ $//')
  case "$pids" in ''|*[!0-9\ ]*) return 1;; esac
  set -- $pids
  [ "$#" -eq 1 ] || return 1
  pid=$1
  [ -r "$STATUS" ] || return 1
  [ "$(status_field pid)" = "$pid" ] || return 1
  fresh_status || return 1
  for expected in "\"commit\": \"$BASE_COMMIT\"" '"adopted": true' '"running": true' '"mode": "GCM"' '"last_status": "Active (GCM)"'; do
    grep -Fq "$expected" "$STATUS" || return 1
  done
  inform=$(sed -n 's/.*"last_successful_inform":[[:space:]]*"\([^"]*\)".*/\1/p' "$STATUS" | head -n 1)
  [ -n "$inform" ]
}

# Recheck identity/health after the small packet transfer and before the cases.
baseline || fail baseline_identity_or_health
[ "$(sha256 "$PAYLOAD/candidate.tar.gz")" = "$ARCHIVE_SHA" ] || fail candidate_archive_hash
[ "$(sha256 "$PAYLOAD/candidate.tar.gz.sha256")" = "$SIDECAR_SHA" ] || fail candidate_sidecar_hash
[ "$(sha256 "$PAYLOAD/candidate-update.sh")" = "$UPDATER_SHA" ] || fail candidate_updater_hash
grep -Fq "$ARCHIVE_SHA" "$PAYLOAD/candidate.tar.gz.sha256" || fail sidecar_content

umask 077
mkdir -m 700 "$STAGE/case-updater" "$STAGE/case-neutral" "$STAGE/case-control" "$STAGE/case-extract" || fail case_directory_creation
: > "$LOG" || fail log_creation

# Emit one sanitized row per sample. File bytes are hashed only; never printed.
observe() {
  label=$1 path=$2 parent=${2%/*} base=${2##*/} sample=$3
  observed_epoch=$(date +%s 2>/dev/null) || observed_epoch=unknown
  state=present kind=other mode=unknown size=unknown digest=unknown
  if [ -L "$path" ]; then
    kind=symlink
  elif [ -f "$path" ]; then
    kind=regular
  elif [ -e "$path" ]; then
    kind=other
  else
    if [ ! -d "$parent" ] || ! ls -1 "$parent" > "$STAGE/observe.list" 2>/dev/null; then
      state=observation_error
    else
      if grep -Fxq "$base" "$STAGE/observe.list"; then
        state=observation_error
      else
        grep_rc=$?
        if [ "$grep_rc" -eq 1 ]; then state=absent_confirmed; else state=observation_error; fi
      fi
    fi
  fi
  if [ "$state" = present ]; then
    metadata=$(ls -lnd "$path" 2>/dev/null) || state=observation_error
    if [ "$state" = present ]; then
      mode=$(printf '%s\n' "$metadata" | awk '{print substr($1,1,10)}') || state=observation_error
      size=$(printf '%s\n' "$metadata" | awk '{print $5}') || state=observation_error
      [ -n "$mode" ] && [ -n "$size" ] || state=observation_error
      if [ "$state" = present ] && [ "$kind" = regular ]; then
        size=$(wc -c < "$path" 2>/dev/null | awk '{print $1}') || state=read_error
        case "$size" in ''|*[!0-9]*) state=read_error;; esac
        if [ "$state" = present ]; then
          digest=$(sha256 "$path") || state=read_error
          case "$digest" in *[!0-9a-f]*|'') state=read_error;; esac
          [ "${#digest}" -eq 64 ] || state=read_error
        fi
      fi
    fi
  fi
  printf 'sample=%s epoch=%s label=%s state=%s type=%s mode=%s size=%s sha256=%s\n' \
    "$sample" "$observed_epoch" "$label" "$state" "$kind" "$mode" "$size" "$digest" >> "$LOG" || fail log_write
  case "$state" in observation_error|read_error) fail "observation_${label}_${sample}_${state}";; esac
}

copy_observe() {
  label=$1 source=$2 destination=$3
  cp -p "$source" "$destination" || fail "create_$label"
  observe "$label" "$destination" immediate
}

copy_observe updater-name "$PAYLOAD/candidate-update.sh" "$STAGE/case-updater/update.sh"
copy_observe neutral-name "$PAYLOAD/candidate-update.sh" "$STAGE/case-neutral/payload.bin"
printf '%s\n' 'harmless non-executable control file' > "$STAGE/case-control/update.sh" || fail create_control
observe control-name "$STAGE/case-control/update.sh" immediate

# Extraction is data-only: no file from the archive is invoked.
if tar -xzpf "$PAYLOAD/candidate.tar.gz" -C "$STAGE/case-extract"; then
  extraction_result=success
else
  extraction_result=error
fi
printf 'extraction=%s\n' "$extraction_result" >> "$LOG" || fail log_write
observe extracted-updater "$STAGE/case-extract/update.sh" immediate
[ "$extraction_result" = success ] || fail candidate_extraction

for sample in 1 5 15 30 60; do
  case "$sample" in 1) sleep 1;; 5) sleep 4;; 15) sleep 10;; 30) sleep 15;; 60) sleep 30;; esac
  observe updater-name "$STAGE/case-updater/update.sh" "$sample"
  observe neutral-name "$STAGE/case-neutral/payload.bin" "$sample"
  observe control-name "$STAGE/case-control/update.sh" "$sample"
  observe extracted-updater "$STAGE/case-extract/update.sh" "$sample"
done

printf 'diagnostic=complete stage=%s observation_seconds=60 evidence=%s\n' "$STAGE" "$LOG"
