#!/bin/sh
# ==============================================================================
# UniFi AP Bridge - Update & Track Management Tool for Asuswrt-Merlin
# ==============================================================================

ADDON_NAME="unifi-bridge"
INSTALL_DIR="/jffs/addons/$ADDON_NAME"
CONFIG_FILE="$INSTALL_DIR/config/config.json"
VERSION_FILE="$INSTALL_DIR/version.txt"
STATUS_JS="/tmp/var/wwwext/$ADDON_NAME/status.js"
UPDATE_JS="/tmp/var/wwwext/$ADDON_NAME/update_status.js"
REPO_RAW="https://raw.githubusercontent.com/agnithin/unifi-ap-bridge"
REPO_RELEASES="https://github.com/agnithin/unifi-ap-bridge/releases"

mkdir -p "/tmp/var/wwwext/$ADDON_NAME"

# Serialize checks and installs so a second request cannot replace active status
# or interleave installed assets. Never steal a stale lock automatically: a PID
# alone cannot prove ownership after PID reuse or a partially completed install.
UPDATE_LOCK="/tmp/unifi-bridge-update.lock"
START_LOCK="/tmp/unifi-bridge-start.lock"
MUTATION_LOCK="/tmp/unifi-bridge-mutation.lock"
acquire_update_lock() {
    if ! (umask 077; mkdir "$UPDATE_LOCK") 2>/dev/null; then
        echo "Another bridge update/check owns $UPDATE_LOCK; retry after it completes. If interrupted, inspect service and lock ownership before recovery." >&2
        return 1
    fi
    if ! echo "$$" > "$UPDATE_LOCK/pid"; then
        rmdir "$UPDATE_LOCK" 2>/dev/null || true
        return 1
    fi
    trap 'release_mutation_lock; release_update_lock' 0
    trap 'exit 130' INT
    trap 'exit 143' TERM
}

acquire_mutation_lock() {
    tries=0
    mutation_wait=${UNIFI_BRIDGE_MUTATION_WAIT_SECONDS:-30}
    while ! (umask 077; mkdir "$MUTATION_LOCK") 2>/dev/null; do
        [ "$tries" -lt "$mutation_wait" ] || { echo "Mutation lock busy; owner $(cat "$MUTATION_LOCK/pid" 2>/dev/null || echo unknown)" >&2; return 1; }
        tries=$((tries + 1)); sleep 1
    done
    echo "$$" > "$MUTATION_LOCK/pid" || { rmdir "$MUTATION_LOCK" 2>/dev/null; return 1; }
    export UNIFI_BRIDGE_MUTATION_LOCK_OWNER="$$"
}

release_mutation_lock() {
    if [ "$(cat "$MUTATION_LOCK/pid" 2>/dev/null)" = "$$" ]; then
        rm -f "$MUTATION_LOCK/pid"
        rmdir "$MUTATION_LOCK" 2>/dev/null || true
    fi
}

acquire_activation_lock() {
    if ! (umask 077; mkdir "$START_LOCK") 2>/dev/null; then
        echo "Another bridge start or activation owns $START_LOCK" >&2
        return 1
    fi
    echo "$$" > "$START_LOCK/pid" || { rmdir "$START_LOCK" 2>/dev/null; return 1; }
    trap 'release_activation_lock; release_mutation_lock; release_update_lock' 0
}

release_activation_lock() {
    if [ "$(cat "$START_LOCK/pid" 2>/dev/null)" = "$$" ]; then
        rm -f "$START_LOCK/pid"
        rmdir "$START_LOCK" 2>/dev/null || true
    fi
}

release_update_lock() {
    if [ "$(cat "$UPDATE_LOCK/pid" 2>/dev/null)" = "$$" ]; then
        rm -f "$UPDATE_LOCK/pid"
        rmdir "$UPDATE_LOCK" 2>/dev/null || true
    fi
}

# Determine current configured branch (default: main)
get_branch() {
    if [ -n "${1:-}" ]; then
        echo "$1"
        return
    fi
    if [ -f "$CONFIG_FILE" ]; then
        b=$(grep '"branch"' "$CONFIG_FILE" 2>/dev/null | head -n 1 | awk -F'"' '{print $4}')
        if [ -n "$b" ]; then
            echo "$b"
            return
        fi
    fi
    echo "main"
}

# Determine current installed version
get_local_version() {
    if [ -f "$VERSION_FILE" ]; then
        v=$(cat "$VERSION_FILE" 2>/dev/null | tr -d ' \t\r\n')
        if [ -n "$v" ]; then
            echo "$v"
            return
        fi
    fi
    if [ -f "$STATUS_JS" ]; then
        v=$(grep '"version":' "$STATUS_JS" 2>/dev/null | head -n 1 | awk -F'"' '{print $4}' | tr -d ' \t\r\n')
        if [ -n "$v" ]; then
            echo "$v"
            return
        fi
    fi
    echo "1.0.0"
}

# Compare semver versions v1 and v2.
# Strips leading 'v' and any prerelease suffix (-dev, etc.).
# Returns:
#   0 if v1 == v2
#   1 if v1 > v2
#   2 if v1 < v2
version_cmp() {
    v1=$(echo "$1" | sed 's/^v//' | sed 's/-.*//' | tr -d ' \t\r\n')
    v2=$(echo "$2" | sed 's/^v//' | sed 's/-.*//' | tr -d ' \t\r\n')
    [ "$v1" = "$v2" ] && return 0

    v1_maj=$(echo "$v1" | awk -F. '{print $1}')
    v1_min=$(echo "$v1" | awk -F. '{print $2}')
    v1_pat=$(echo "$v1" | awk -F. '{print $3}')
    v2_maj=$(echo "$v2" | awk -F. '{print $1}')
    v2_min=$(echo "$v2" | awk -F. '{print $2}')
    v2_pat=$(echo "$v2" | awk -F. '{print $3}')

    [ -z "$v1_maj" ] && v1_maj=0
    [ -z "$v1_min" ] && v1_min=0
    [ -z "$v1_pat" ] && v1_pat=0
    [ -z "$v2_maj" ] && v2_maj=0
    [ -z "$v2_min" ] && v2_min=0
    [ -z "$v2_pat" ] && v2_pat=0

    [ "$v1_maj" -gt "$v2_maj" ] 2>/dev/null && return 1
    [ "$v1_maj" -lt "$v2_maj" ] 2>/dev/null && return 2
    [ "$v1_min" -gt "$v2_min" ] 2>/dev/null && return 1
    [ "$v1_min" -lt "$v2_min" ] 2>/dev/null && return 2
    [ "$v1_pat" -gt "$v2_pat" ] 2>/dev/null && return 1
    [ "$v1_pat" -lt "$v2_pat" ] 2>/dev/null && return 2
    return 0
}

get_installed_branch() {
    installed=$(grep '"branch"' "$STATUS_JS" 2>/dev/null | head -n 1 | awk -F'"' '{print $4}')
    [ -n "$installed" ] || installed=$(get_branch)
    echo "$installed"
}

# Write update_status.js for WebUI consumption
write_update_status() {
    status="$1"
    branch="$2"
    cur_ver="$3"
    rem_ver="$4"
    msg="$5"
    now=$(date +"%Y-%m-%d %H:%M:%S")

    cat << EOF > "$UPDATE_JS.tmp"
window.unifiBridgeUpdate = {
  "status": "${status}",
  "branch": "${branch}",
  "installed_branch": "$(get_installed_branch)",
  "operation": "${operation:-check}",
  "request_id": "${request_id:-}",
  "installed_newer": ${installed_newer:-false},
  "current_version": "${cur_ver}",
  "remote_version": "${rem_ver}",
  "message": "${msg}",
  "last_check": "${now}"
};
EOF
    mv -f "$UPDATE_JS.tmp" "$UPDATE_JS"
    chmod 644 "$UPDATE_JS"
    if [ "${operation:-}" = "cluster_update" ] && { [ "$status" = "SUCCESS" ] || [ "$status" = "ERROR" ]; }; then
        operation_status="FAILED"
        [ "$status" = "SUCCESS" ] && operation_status="APPLIED"
        operation_id=$(printf '%s' "${request_id:-}" | sed 's/[^0-9]//g')
        operation_commit=$(printf '%s' "${resolved_commit:-}" | sed 's/[^0-9a-f]//g')
        cat > "$UPDATE_JS.cluster.tmp" << EOF
window.unifiBridgeClusterOperation = {"operation_id":"${operation_id}","operation":"update","status":"${operation_status}","version":"${rem_ver}","commit":"${operation_commit}"};
EOF
        mv -f "$UPDATE_JS.cluster.tmp" "/tmp/var/wwwext/$ADDON_NAME/cluster_operation.js"
        chmod 644 "/tmp/var/wwwext/$ADDON_NAME/cluster_operation.js"
    fi
}

managed_assets="unifi-bridge.tar.gz
unifi-bridge.tar.gz.sha256
start.sh
apply_config.sh
sync_nodes.sh
unifi-bridge.sh
update.sh
watchdog.sh
version.txt
webui/unifi-bridge.asp"
TXN_DIR="$INSTALL_DIR/.update-transaction"

begin_transaction() {
    [ ! -e "$TXN_DIR" ] || { echo "Unfinished addon transaction exists; inspect $TXN_DIR" >&2; return 1; }
    (umask 077; mkdir -p "$TXN_DIR/snapshot") || return 1
    : > "$TXN_DIR/manifest" || return 1
    printf '%s\n' "$managed_assets" | while IFS= read -r rel; do
        [ -n "$rel" ] || continue
        if [ -f "$INSTALL_DIR/$rel" ]; then
            mkdir -p "$TXN_DIR/snapshot/$(dirname "$rel")" || exit 1
            cp -p "$INSTALL_DIR/$rel" "$TXN_DIR/snapshot/$rel" || exit 1
            printf 'present %s\n' "$rel" >> "$TXN_DIR/manifest" || exit 1
        else
            printf 'absent %s\n' "$rel" >> "$TXN_DIR/manifest" || exit 1
        fi
    done || { rm -rf "$TXN_DIR"; return 1; }
    prior_member="unifi-bridge-${ARCH_SUFFIX}"
    prior_dir="$tmp_dir/prior-snapshot"
    mkdir -p "$prior_dir" || { rm -rf "$TXN_DIR"; return 1; }
    if [ ! -f "$TXN_DIR/snapshot/unifi-bridge.tar.gz" ] || [ ! -f "$TXN_DIR/snapshot/unifi-bridge.tar.gz.sha256" ] ||
       ! tar -tzf "$TXN_DIR/snapshot/unifi-bridge.tar.gz" > "$tmp_dir/prior-members" 2>/dev/null ||
       ! tar -tvzf "$TXN_DIR/snapshot/unifi-bridge.tar.gz" > "$tmp_dir/prior-types" 2>/dev/null; then
        echo "No extractable prior addon generation is available for safe rollback" >&2
        rm -rf "$TXN_DIR"; return 1
    fi
    prior_listing=$(LC_ALL=C sort "$tmp_dir/prior-members")
    prior_complete="apply_config.sh
bundle.json
install.sh
start.sh
sync_nodes.sh
${prior_member}
unifi-bridge.asp
unifi-bridge.sh
update.sh
version.txt
watchdog.sh"
    if [ "$prior_listing" != "$prior_complete" ]; then
        echo "Prior archive member set is not an approved addon generation" >&2
        rm -rf "$TXN_DIR"; return 1
    fi
    prior_expected=$(awk -v name="$(basename "$TXN_DIR/snapshot/unifi-bridge.tar.gz")" 'NF==2 && $2==name {print $1}' "$TXN_DIR/snapshot/unifi-bridge.tar.gz.sha256")
    prior_actual=$(openssl dgst -sha256 "$TXN_DIR/snapshot/unifi-bridge.tar.gz" 2>/dev/null | awk '{print $NF}')
    [ "${#prior_expected}" -eq 64 ] && [ "$prior_expected" = "$prior_actual" ] || { echo "Prior generation checksum is missing or invalid" >&2; rm -rf "$TXN_DIR"; return 1; }
    prior_invalid_types=$(awk 'substr($0,1,1)!="-" || substr($0,6,1)=="w" || substr($0,9,1)=="w" || substr($0,4,1) ~ /[sS]/ || substr($0,7,1) ~ /[sS]/ || substr($0,10,1) ~ /[tT]/ { print "invalid"; exit } END { if (!NR) print "invalid" }' "$tmp_dir/prior-types")
    if [ -n "$prior_invalid_types" ] || ! tar -xzf "$TXN_DIR/snapshot/unifi-bridge.tar.gz" -C "$prior_dir" >/dev/null 2>&1; then
        echo "Prior archive contains unsafe member types or could not be extracted" >&2
        rm -rf "$TXN_DIR"; return 1
    fi
    prior_binary="$prior_dir/$prior_member"
    if [ ! -f "$prior_binary" ]; then
        echo "Prior addon archive has no binary for this architecture" >&2
        rm -rf "$TXN_DIR"; return 1
    fi
    prior_identity=$("$prior_binary" -version 2>/dev/null) || { rm -rf "$TXN_DIR"; return 1; }
    prior_version=$(printf '%s' "$prior_identity" | sed -n 's/^unifi-bridge \([^ ]*\).*/\1/p')
    prior_commit=$(printf '%s' "$prior_identity" | sed -n 's/.*commit: \([^,]*\),.*/\1/p')
    prior_meta_version=$(sed -n 's/^[[:space:]]*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$prior_dir/bundle.json")
    prior_meta_commit=$(sed -n 's/^[[:space:]]*"commit"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$prior_dir/bundle.json")
    prior_meta_arch=$(sed -n 's/^[[:space:]]*"architecture"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$prior_dir/bundle.json")
    [ "$prior_meta_version" = "$prior_version" ] && [ "$prior_meta_commit" = "$prior_commit" ] && [ "$prior_meta_arch" = "$ARCH_SUFFIX" ] || { echo "Prior bundle metadata and executable identity disagree" >&2; rm -rf "$TXN_DIR"; return 1; }
    prior_digest=$(openssl dgst -sha256 "$prior_binary" 2>/dev/null | awk '{print $NF}')
    live_digest=$(openssl dgst -sha256 /tmp/unifi-bridge/unifi-bridge 2>/dev/null | awk '{print $NF}')
    [ -n "$prior_digest" ] && [ "$prior_digest" = "$live_digest" ] || { echo "Prior archive binary does not match the current runtime executable" >&2; rm -rf "$TXN_DIR"; return 1; }
    [ "$prior_version" = "$(cat "$TXN_DIR/snapshot/version.txt" 2>/dev/null | tr -d ' \t\r\n')" ] || { echo "Prior bundle and installed version disagree" >&2; rm -rf "$TXN_DIR"; return 1; }
    case "$prior_commit" in *[!0-9a-f]*|'') prior_commit="" ;; esac
    [ "${#prior_commit}" -eq 40 ] || { echo "Prior bundle source identity is not verifiable" >&2; rm -rf "$TXN_DIR"; return 1; }
    for rel in start.sh apply_config.sh sync_nodes.sh unifi-bridge.sh update.sh watchdog.sh; do
        [ "$(openssl dgst -sha256 "$TXN_DIR/snapshot/$rel" 2>/dev/null | awk '{print $NF}')" = "$(openssl dgst -sha256 "$prior_dir/$rel" 2>/dev/null | awk '{print $NF}')" ] || { echo "Prior installed script set does not match its bundle" >&2; rm -rf "$TXN_DIR"; return 1; }
    done
    [ "$(openssl dgst -sha256 "$TXN_DIR/snapshot/webui/unifi-bridge.asp" 2>/dev/null | awk '{print $NF}')" = "$(openssl dgst -sha256 "$prior_dir/unifi-bridge.asp" 2>/dev/null | awk '{print $NF}')" ] || { echo "Prior installed WebUI does not match its bundle" >&2; rm -rf "$TXN_DIR"; return 1; }
    if "$prior_binary" -config "$CONFIG_FILE" -is-enabled; then
        prior_status=$(cat "$STATUS_JS" 2>/dev/null)
        prior_pids=$(pidof unifi-bridge 2>/dev/null | tr -s ' ' | sed 's/^ //;s/ $//')
        prior_pid=$(printf '%s' "$prior_status" | sed -n 's/.*"pid":[[:space:]]*\([0-9][0-9]*\).*/\1/p' | head -n 1)
        prior_updated=$(printf '%s' "$prior_status" | sed -n 's/.*"status_updated_at":[[:space:]]*\([0-9][0-9]*\).*/\1/p' | head -n 1)
        prior_now=$(date +%s)
        prior_exe=$(readlink "/proc/$prior_pid/exe" 2>/dev/null)
        prior_inform=$(printf '%s' "$prior_status" | sed -n 's/.*"last_successful_inform":[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)
        [ -n "$prior_pids" ] && ! printf '%s' "$prior_pids" | grep -q ' ' && [ "$prior_pids" = "$prior_pid" ] &&
            [ "$prior_exe" = "/tmp/unifi-bridge/unifi-bridge" ] && [ -n "$prior_inform" ] &&
            [ "$prior_updated" -le "$prior_now" ] 2>/dev/null && [ $((prior_now - prior_updated)) -le 180 ] 2>/dev/null &&
            printf '%s' "$prior_status" | grep -Fq "\"version\": \"$prior_version\"" &&
            printf '%s' "$prior_status" | grep -Fq "\"commit\": \"$prior_commit\"" &&
            printf '%s' "$prior_status" | grep -Fq '"last_status": "Active (' || {
                echo "Prior running addon identity is not currently verified" >&2; rm -rf "$TXN_DIR"; return 1;
            }
    else
        prior_enabled_rc=$?
        prior_status=$(cat "$STATUS_JS" 2>/dev/null)
        prior_pids=$(pidof unifi-bridge 2>/dev/null | tr -d ' \t\r\n')
        # Disabled status is static and the watchdog intentionally does not
        # refresh it. The validated installed helper plus absence of a process
        # prove disabled state; freshness is required only after candidate start.
        [ "$prior_enabled_rc" -eq 3 ] && [ -z "$prior_pids" ] &&
            printf '%s' "$prior_status" | grep -Eq '"enabled"[[:space:]]*:[[:space:]]*false' &&
            printf '%s' "$prior_status" | grep -Eq '"running"[[:space:]]*:[[:space:]]*false' &&
            printf '%s' "$prior_status" | grep -Eq "\"version\"[[:space:]]*:[[:space:]]*\"$prior_version\"" &&
            printf '%s' "$prior_status" | grep -Eq "\"commit\"[[:space:]]*:[[:space:]]*\"$prior_commit\"" || {
                echo "Prior disabled addon identity is not currently verified" >&2; rm -rf "$TXN_DIR"; return 1;
            }
    fi
    sync
    printf 'ACTIVATING\n' > "$TXN_DIR/active.tmp" || return 1
    mv -f "$TXN_DIR/active.tmp" "$TXN_DIR/active" || return 1
    sync
}

atomic_install() {
    source_file="$1"; dest_file="$2"; mode="$3"
    dest_dir=$(dirname "$dest_file")
    mkdir -p "$dest_dir" || return 1
    temp_file="$dest_file.update.$$"
    rm -f "$temp_file"
    cp -f "$source_file" "$temp_file" && chmod "$mode" "$temp_file" && mv -f "$temp_file" "$dest_file"
    result=$?
    [ "$result" -eq 0 ] || rm -f "$temp_file"
    return "$result"
}

restore_transaction() {
    [ -f "$TXN_DIR/active" ] || return 1
    UNIFI_BRIDGE_START_LOCK_OWNER="$$" UNIFI_BRIDGE_MUTATION_LOCK_OWNER="$$" "$INSTALL_DIR/start.sh" --recover-only || return 1
    return 0
}

clear_transaction() {
    complete="$INSTALL_DIR/.update-complete-$$"
    mv "$TXN_DIR" "$complete" || return 1
    sync
    rm -rf "$complete"
}

restore_prior_entry_and_clear() {
    atomic_install "$TXN_DIR/snapshot/start.sh" "$INSTALL_DIR/start.sh" 755 || return 1
    sync
    clear_transaction
}

rollback_and_verify() {
    reason="$1"
    rollback_started_at=$(date +%s)
    write_update_status "ERROR" "$branch" "$local_ver" "${remote_ver:-}" "$reason; attempting verified rollback"
    if ! restore_transaction; then
        write_update_status "ERROR" "$branch" "$local_ver" "${remote_ver:-}" "$reason; ROLLBACK FAILED: prior addon assets could not be restored; journal retained"
        return 1
    fi
    if ! UNIFI_BRIDGE_START_LOCK_OWNER="$$" UNIFI_BRIDGE_MUTATION_LOCK_OWNER="$$" "$INSTALL_DIR/start.sh"; then
        write_update_status "ERROR" "$branch" "$local_ver" "${remote_ver:-}" "$reason; ROLLBACK UNVERIFIED: prior assets restored but startup failed"
        return 1
    fi
    # Keep it simple and conservative: verify the prior process remains present
    # and its status belongs to the prior generation. Disabled installs must
    # publish disabled metadata and leave no bridge process.
    prior_commit=$(sed -n 's/.*commit: \([^,]*\),.*/\1/p' "$TXN_DIR/snapshot/unifi-bridge.tar.gz" 2>/dev/null)
    # The old executable is authoritative for a runnable prior generation.
    prior_bin="$INSTALL_DIR/unifi-bridge.tar.gz"
    if [ -f "$prior_bin" ]; then
        mkdir -p "$tmp_dir/prior" && tar -xzf "$prior_bin" -C "$tmp_dir/prior" >/dev/null 2>&1 || {
            write_update_status "ERROR" "$branch" "$local_ver" "${remote_ver:-}" "$reason; ROLLBACK UNVERIFIED: prior bundle could not be extracted"
            return 1
        }
        prior_member="unifi-bridge-${ARCH_SUFFIX}"
        prior_version=$($tmp_dir/prior/"$prior_member" -version 2>/dev/null)
        prior_commit=$(printf '%s' "$prior_version" | sed -n 's/.*commit: \([^,]*\),.*/\1/p')
        prior_ver=$(printf '%s' "$prior_version" | sed -n 's/^unifi-bridge \([^ ]*\).*/\1/p')
    else
        prior_commit=""; prior_ver=""
    fi
    if [ -n "$prior_commit" ] && [ -n "$prior_ver" ]; then
        if $tmp_dir/prior/"$prior_member" -config "$CONFIG_FILE" -is-enabled; then
            verify_live "$prior_ver" "$prior_commit" "$rollback_started_at"
            if [ "$verified" -ne 1 ]; then
                write_update_status "ERROR" "$branch" "$local_ver" "${remote_ver:-}" "$reason; ROLLBACK UNVERIFIED: prior assets restored but matching fresh heartbeat was not observed"
                return 1
            fi
        else
            prior_enabled_rc=$?
            live_pids=$(pidof unifi-bridge 2>/dev/null | tr -d ' \t\r\n')
            status_snapshot=$(cat "$STATUS_JS" 2>/dev/null)
            if [ "$prior_enabled_rc" -ne 3 ] || [ -n "$live_pids" ] ||
               ! printf '%s' "$status_snapshot" | grep -Eq '"enabled"[[:space:]]*:[[:space:]]*false' ||
               ! printf '%s' "$status_snapshot" | grep -Eq '"running"[[:space:]]*:[[:space:]]*false' ||
               ! printf '%s' "$status_snapshot" | grep -Eq "\"version\"[[:space:]]*:[[:space:]]*\"$prior_ver\"" ||
               ! printf '%s' "$status_snapshot" | grep -Eq "\"commit\"[[:space:]]*:[[:space:]]*\"$prior_commit\""; then
                write_update_status "ERROR" "$branch" "$local_ver" "${remote_ver:-}" "$reason; ROLLBACK UNVERIFIED: disabled prior state was not confirmed"
                return 1
            fi
        fi
    else
        # A previously disabled installation has no resident daemon.
        live_pids=$(pidof unifi-bridge 2>/dev/null | tr -d ' \t\r\n')
        status_snapshot=$(cat "$STATUS_JS" 2>/dev/null)
        case "$live_pids" in ''|*[!0-9]*) ;; *) live_pids="invalid" ;; esac
        if [ -n "$live_pids" ] || ! printf '%s' "$status_snapshot" | grep -Fq '"enabled":false' ||
           ! printf '%s' "$status_snapshot" | grep -Fq '"running":false'; then
            write_update_status "ERROR" "$branch" "$local_ver" "${remote_ver:-}" "$reason; ROLLBACK UNVERIFIED: disabled prior state could not be confirmed"
            return 1
        fi
    fi
    if ! restore_prior_entry_and_clear; then
        write_update_status "ERROR" "$branch" "$local_ver" "${remote_ver:-}" "$reason; prior process verified but recovery entry/journal cleanup failed"
        return 1
    fi
    write_update_status "ERROR" "$branch" "$local_ver" "${remote_ver:-}" "$reason; previous addon generation restored and verified"
    return 1
}

verify_live() {
    want_version="$1"; want_commit="$2"; since="$3"
    attempts=0; verified=0
    while [ "$attempts" -lt 30 ]; do
        pid_list=$(pidof unifi-bridge 2>/dev/null | tr -s ' ' | sed 's/^ //;s/ $//')
        case "$pid_list" in ''|*[!0-9\ ]*) pid_list="invalid" ;; esac
        case "$pid_list" in *' '*) pid_list="multiple" ;; esac
        status_snapshot=$(cat "$STATUS_JS" 2>/dev/null)
        live_pid="$pid_list"
        updated=$(printf '%s\n' "$status_snapshot" | sed -n 's/.*"status_updated_at":[[:space:]]*\([0-9][0-9]*\).*/\1/p' | head -n 1)
        status_pid=$(printf '%s\n' "$status_snapshot" | sed -n 's/.*"pid":[[:space:]]*\([0-9][0-9]*\).*/\1/p' | head -n 1)
        process_exe=$(readlink "/proc/$live_pid/exe" 2>/dev/null)
        last_inform=$(printf '%s\n' "$status_snapshot" | sed -n 's/.*"last_successful_inform":[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)
        case "$live_pid:$status_pid:$updated" in *[!0-9:]*|:*|*:) ;; *)
            if [ "$live_pid" = "$status_pid" ] && [ "$updated" -ge "$since" ] &&
               printf '%s\n' "$status_snapshot" | grep -Fq "\"version\": \"$want_version\"" &&
               printf '%s\n' "$status_snapshot" | grep -Fq "\"commit\": \"$want_commit\"" &&
               printf '%s\n' "$status_snapshot" | grep -Fq '"running": true' &&
               printf '%s\n' "$status_snapshot" | grep -Fq '"enabled": true' &&
               [ "$process_exe" = "/tmp/unifi-bridge/unifi-bridge" ] &&
               [ -n "$last_inform" ] &&
               printf '%s\n' "$status_snapshot" | grep -Fq '"last_status": "Active ('; then
                verified=1; return 0
            fi ;;
        esac
        attempts=$((attempts + 1)); sleep 2
    done
    return 1
}

cmd_check() {
    operation="check"
    request_id="${2:-}"
    installed_newer=false
    branch=$(get_branch "${1:-}")
    case "$branch" in main|dev) ;; *) echo "Invalid release track" >&2; return 1 ;; esac
    local_ver=$(get_local_version)
    echo "Checking for updates on branch '${branch}' (current version: ${local_ver})..."

    write_update_status "CHECKING" "$branch" "$local_ver" "-" "Checking GitHub repository..."

    raw_ver=""
    if [ "$branch" = "main" ]; then
        # Query latest official release tag via redirect (instant, zero rate limit)
        rel_tag=$(curl -sI --connect-timeout 4 --max-time 6 "${REPO_RELEASES}/latest" 2>/dev/null | grep -i '^Location:' | awk -F'/' '{print $NF}' | tr -d ' \t\r\n' || true)
        if [ -n "$rel_tag" ]; then
            raw_ver="$rel_tag"
        fi
    fi

    if [ -z "$raw_ver" ]; then
        raw_ver=$(curl -fsSL --connect-timeout 5 --max-time 10 "${REPO_RAW}/${branch}/version.txt" 2>/dev/null | tr -d ' \t\r\n' || true)
    fi

    if [ -z "$raw_ver" ]; then
        echo "Error: Failed to fetch version info from GitHub"
        write_update_status "ERROR" "$branch" "$local_ver" "-" "Unable to reach GitHub to check version"
        return 1
    fi

    # Strip leading 'v' if present for normalized comparison
    clean_local=$(echo "$local_ver" | sed 's/^v//')
    clean_remote=$(echo "$raw_ver" | sed 's/^v//')

    local_ver="$clean_local"
    raw_ver="$clean_remote"
    installed_track=$(get_installed_branch)
    version_cmp "$clean_local" "$clean_remote"
    cmp_res=$?

    if [ "$cmp_res" -eq 0 ]; then
        echo "UniFi AP Bridge is up to date (version: ${raw_ver}, branch: ${branch})"
        write_update_status "UP_TO_DATE" "$branch" "$local_ver" "$raw_ver" "Installed v${local_ver} (${installed_track}) matches latest v${raw_ver} (${branch})"
    elif [ "$cmp_res" -eq 1 ]; then
        echo "Installed version (${local_ver}) is newer than remote (${raw_ver}) on branch '${branch}'"
        installed_newer=true
        write_update_status "UP_TO_DATE" "$branch" "$local_ver" "$raw_ver" "Installed v${local_ver} (${installed_track}) is newer than latest v${raw_ver} (${branch})"
    else
        echo "New version available: ${raw_ver} (installed: ${local_ver})"
        write_update_status "UPDATE_AVAILABLE" "$branch" "$local_ver" "$raw_ver" "Latest v${raw_ver} (${branch}) is newer than installed v${local_ver} (${installed_track})"
    fi
    return 0
}

cmd_apply() {
    operation="apply"
    request_id="${2:-}"
    exact_commit="${3:-}"
    [ -z "$exact_commit" ] || operation="cluster_update"
    installed_newer=false
    branch=$(get_branch "${1:-}")
    case "$branch" in main|dev) ;; *) echo "Invalid release track" >&2; return 1 ;; esac
    local_ver=$(get_local_version)
    tmp_dir="/tmp/unifi-update-$$"
    umask 077

    echo "====================================================="
    echo " Starting update on track '${branch}'..."
    echo "====================================================="

    write_update_status "UPDATING" "$branch" "$local_ver" "-" "Downloading update files from track '${branch}'..."

    mkdir -p "$tmp_dir"
    rm -rf "${tmp_dir:?}"/*

    # Detect only architectures produced by CI; never substitute another build.
    ARCH=$(uname -m)
    case "$ARCH" in
        armv7l) ARCH_SUFFIX="armv7" ;;
        aarch64) ARCH_SUFFIX="arm64" ;;
        *) write_update_status "ERROR" "$branch" "$local_ver" "-" "Unsupported device architecture; installed files were not changed"
           rm -rf "$tmp_dir"; return 1 ;;
    esac

    # Resolve one immutable release generation. A cluster exact-commit apply
    # uses the same dev tag, or proves the current stable tag still names the
    # requested commit before fetching its asset.
    case "$exact_commit" in *[!0-9a-f]*|'') exact_commit="" ;; esac
    [ -z "$exact_commit" ] || [ "${#exact_commit}" -eq 40 ] || exact_commit=""
    if [ -n "${3:-}" ] && [ -z "$exact_commit" ]; then
        write_update_status "ERROR" "$branch" "$local_ver" "-" "Requested exact source commit is invalid; installed files were not changed"
        rm -rf "$tmp_dir"; return 1
    fi
    if [ "$branch" = "dev" ]; then
        if [ -n "$exact_commit" ]; then
            resolved_commit="$exact_commit"
        else
            resolved_commit=$(curl -fsSL --connect-timeout 8 --max-time 20 \
                -H 'Accept: application/vnd.github+json' \
                'https://api.github.com/repos/agnithin/unifi-ap-bridge/commits/dev' 2>/dev/null |
                awk -F'"' '/"sha"[[:space:]]*:/ { print $4; exit }')
        fi
        case "$resolved_commit" in *[!0-9a-f]*|'') resolved_commit="" ;; esac
        [ "${#resolved_commit}" -eq 40 ] || resolved_commit=""
        release_tag="dev-${resolved_commit}"
        expected_version=$(curl -fsSL --connect-timeout 8 --max-time 20 \
            "${REPO_RAW}/${resolved_commit}/version.txt" 2>/dev/null | tr -d ' \t\r\n')
    else
        release_tag=$(curl -fsSL --connect-timeout 8 --max-time 20 \
            -H 'Accept: application/vnd.github+json' \
            'https://api.github.com/repos/agnithin/unifi-ap-bridge/releases/latest' 2>/dev/null |
            sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)
        resolved_commit=$(curl -fsSL --connect-timeout 8 --max-time 20 \
            -H 'Accept: application/vnd.github+json' \
            "https://api.github.com/repos/agnithin/unifi-ap-bridge/commits/${release_tag}" 2>/dev/null |
            awk -F'"' '/"sha"[[:space:]]*:/ { print $4; exit }')
        expected_version=${release_tag#v}
        if [ -n "$exact_commit" ] && [ "$resolved_commit" != "$exact_commit" ]; then
            write_update_status "ERROR" "$branch" "$local_ver" "-" "Stable release no longer names the requested source commit; installed files were not changed"
            rm -rf "$tmp_dir"; return 1
        fi
    fi
    case "$resolved_commit" in *[!0-9a-f]*|'') resolved_commit="" ;; esac
    [ "${#resolved_commit}" -eq 40 ] || resolved_commit=""
    case "$release_tag" in ''|*[!A-Za-z0-9._-]*)
        write_update_status "ERROR" "$branch" "$local_ver" "-" "Could not resolve a valid immutable release; installed files were not changed"
        rm -rf "$tmp_dir"; return 1 ;;
    esac
    release_url="${REPO_RELEASES}/download/${release_tag}"
    bundle_name="unifi-bridge-addon-${resolved_commit}-${ARCH_SUFFIX}.tar.gz"
    bundle_archive="${tmp_dir}/${bundle_name}"
    checksum_file="${tmp_dir}/${bundle_name}.sha256"
    if ! curl -fsSL --connect-timeout 10 --max-time 60 "${release_url}/${bundle_name}" -o "$bundle_archive" 2>/dev/null ||
       ! curl -fsSL --connect-timeout 8 --max-time 20 "${release_url}/${bundle_name}.sha256" -o "$checksum_file" 2>/dev/null; then
        write_update_status "ERROR" "$branch" "$local_ver" "-" "Complete addon bundle unavailable for resolved release; installed files were not changed"
        rm -rf "$tmp_dir"; return 1
    fi
    checksum_rows=$(awk 'NF { n++ } END { print n+0 }' "$checksum_file")
    expected=$(awk -v name="$bundle_name" 'NF==2 && $2==name {print $1}' "$checksum_file")
    actual=$(openssl dgst -sha256 "$bundle_archive" 2>/dev/null | awk '{print $NF}')
    case "$expected" in *[!0-9a-f]*|'') expected="" ;; esac
    if [ "$checksum_rows" -ne 1 ] || [ "${#expected}" -ne 64 ] || [ "$expected" != "$actual" ]; then
        write_update_status "ERROR" "$branch" "$local_ver" "-" "Bundle checksum mismatch; installed files were not changed"
        rm -rf "$tmp_dir"; return 1
    fi
    # Persist a normalized manifest beside the installed local archive. Release
    # manifests name the immutable download artifact; recovery uses the local
    # name and must be able to validate every subsequently installed generation.
    installed_checksum_file="${tmp_dir}/installed-bundle.sha256"
    printf '%s  unifi-bridge.tar.gz\n' "$expected" > "$installed_checksum_file" || {
        write_update_status "ERROR" "$branch" "$local_ver" "-" "Could not stage normalized bundle checksum; installed files were not changed"
        rm -rf "$tmp_dir"; return 1
    }
    binary_member="unifi-bridge-${ARCH_SUFFIX}"
    expected_members="apply_config.sh
bundle.json
install.sh
start.sh
sync_nodes.sh
${binary_member}
unifi-bridge.asp
unifi-bridge.sh
update.sh
version.txt
watchdog.sh"
    if ! tar -tzf "$bundle_archive" > "$tmp_dir/members" 2>/dev/null ||
       ! tar -tvzf "$bundle_archive" > "$tmp_dir/member_types" 2>/dev/null; then
        write_update_status "ERROR" "$branch" "$local_ver" "-" "Bundle archive listing failed; installed files were not changed"
        rm -rf "$tmp_dir"; return 1
    fi
    contents=$(LC_ALL=C sort "$tmp_dir/members")
    if [ "$contents" != "$expected_members" ]; then
        write_update_status "ERROR" "$branch" "$local_ver" "-" "Unexpected bundle paths or members; installed files were not changed"
        rm -rf "$tmp_dir"; return 1
    fi
    types=$(awk 'substr($0,1,1)!="-" || substr($0,6,1)=="w" || substr($0,9,1)=="w" || substr($0,4,1) ~ /[sS]/ || substr($0,7,1) ~ /[sS]/ || substr($0,10,1) ~ /[tT]/ { print "invalid"; exit } END { if (!NR) print "invalid" }' "$tmp_dir/member_types")
    if [ -n "$types" ]; then
        write_update_status "ERROR" "$branch" "$local_ver" "-" "Bundle contains a non-regular member; installed files were not changed"
        rm -rf "$tmp_dir"; return 1
    fi
    mkdir -p "$tmp_dir/staged"
    if ! tar -xzf "$bundle_archive" -C "$tmp_dir/staged"; then
        write_update_status "ERROR" "$branch" "$local_ver" "-" "Bundle extraction failed; installed files were not changed"
        rm -rf "$tmp_dir"; return 1
    fi
    remote_ver=$(sed -n 's/^[[:space:]]*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$tmp_dir/staged/bundle.json")
    staged_commit=$(sed -n 's/^[[:space:]]*"commit"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$tmp_dir/staged/bundle.json")
    staged_arch=$(sed -n 's/^[[:space:]]*"architecture"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$tmp_dir/staged/bundle.json")
    staged_version=$("$tmp_dir/staged/$binary_member" -version 2>/dev/null)
    staged_version_status=$?
    case "$staged_commit" in *[!0-9a-f]*|'') staged_commit="" ;; esac
    [ "${#staged_commit}" -eq 40 ] || staged_commit=""
    expected_binary_prefix="unifi-bridge ${remote_ver} (commit: ${staged_commit},"
    case "$staged_version" in
        "$expected_binary_prefix"*) ;;
        *) staged_commit="" ;;
    esac
    if [ "$staged_version_status" -ne 0 ] || [ -z "$remote_ver" ] || [ "$remote_ver" != "$expected_version" ] || [ -z "$staged_commit" ] || [ "$staged_arch" != "$ARCH_SUFFIX" ] ||
       [ "$(cat "$tmp_dir/staged/version.txt" 2>/dev/null | tr -d ' \t\r\n')" != "$remote_ver" ] ||
       [ "$staged_commit" != "$resolved_commit" ] ||
       { [ "$branch" = "main" ] && [ "v${remote_ver}" != "$release_tag" ]; }; then
        write_update_status "ERROR" "$branch" "$local_ver" "-" "Bundle metadata, version, architecture or source commit did not match the resolved release; installed files were not changed"
        rm -rf "$tmp_dir"; return 1
    fi
    if ! acquire_mutation_lock; then
        write_update_status "ERROR" "$branch" "$local_ver" "-" "Another settings, sync or addon mutation is active; installed files were not changed"
        rm -rf "$tmp_dir"; return 1
    fi
    if ! acquire_activation_lock; then
        write_update_status "ERROR" "$branch" "$local_ver" "$remote_ver" "Another bridge start or activation is active; installed files were not changed"
        rm -rf "$tmp_dir"; return 1
    fi
    if [ -e "$TXN_DIR" ]; then
        write_update_status "ERROR" "$branch" "$local_ver" "-" "Unfinished addon update journal requires recovery before mutation"
        rm -rf "$tmp_dir"; return 1
    fi
    if ! begin_transaction; then
        write_update_status "ERROR" "$branch" "$local_ver" "$remote_ver" "Could not persist previous addon generation; installed files were not changed"
        rm -rf "$tmp_dir"; return 1
    fi
    if ! chmod 755 "$INSTALL_DIR" || ! mkdir -p "$INSTALL_DIR/.ssh" || ! chmod 700 "$INSTALL_DIR/.ssh"; then
        rm -rf "$TXN_DIR"
        write_update_status "ERROR" "$branch" "$local_ver" "$remote_ver" "Could not prepare addon permissions; managed assets were not changed"
        rm -rf "$tmp_dir"; return 1
    fi

    # 3. Install the recovery entry point first, then every asset through a
    # same-filesystem temporary file and rename. Mutable config/state/keys are
    # outside the managed generation and are never rewritten by this update.
    echo "[*] Installing updated scripts to ${INSTALL_DIR}..."
    if ! atomic_install "$tmp_dir/staged/start.sh" "$INSTALL_DIR/start.sh" 755; then
        rm -rf "$TXN_DIR"
        write_update_status "ERROR" "$branch" "$local_ver" "$remote_ver" "Recovery startup rename failed before generation mutation; prior files remain active"
        rm -rf "$tmp_dir"; return 1
    fi
    sync
    for f in start.sh apply_config.sh sync_nodes.sh unifi-bridge.sh update.sh watchdog.sh; do
        [ "$f" = start.sh ] && continue
        if ! atomic_install "${tmp_dir}/staged/$f" "$INSTALL_DIR/$f" 755; then
            rollback_and_verify "Script installation failed at $f"; rm -rf "$tmp_dir"; return 1
        fi
    done
    mkdir -p "$INSTALL_DIR/webui"
    slot="user3"
    [ -f "$INSTALL_DIR/.user_slot" ] && slot=$(cat "$INSTALL_DIR/.user_slot" 2>/dev/null | tr -d ' \t\r\n')
    [ -z "$slot" ] && slot="user3"
    if ! atomic_install "${tmp_dir}/staged/version.txt" "$INSTALL_DIR/version.txt" 644 ||
       ! atomic_install "${tmp_dir}/staged/unifi-bridge.asp" "$INSTALL_DIR/webui/unifi-bridge.asp" 644 ||
       ! atomic_install "$bundle_archive" "$INSTALL_DIR/unifi-bridge.tar.gz" 644 ||
       ! atomic_install "$installed_checksum_file" "$INSTALL_DIR/unifi-bridge.tar.gz.sha256" 644 ||
       ! atomic_install "${tmp_dir}/staged/unifi-bridge.asp" "/tmp/var/wwwext/${slot}.asp" 644; then
        rollback_and_verify "Persistent or live UI installation failed"; rm -rf "$tmp_dir"; return 1
    fi

    # 4. Restart the local daemon and validate this exact invocation.
    echo "[*] Restarting UniFi AP Bridge daemon..."
    started_at=$(date +%s)
    if ! UNIFI_BRIDGE_START_LOCK_OWNER="$$" UNIFI_BRIDGE_MUTATION_LOCK_OWNER="$$" "$INSTALL_DIR/start.sh"; then
        rollback_and_verify "Candidate bridge startup failed"; rm -rf "$tmp_dir"; return 1
    fi

    # A shell launch is not proof that the daemon is running the new binary or
    # exchanging informs. Disabled installs intentionally have no heartbeat.
    if "$tmp_dir/staged/unifi-bridge-${ARCH_SUFFIX}" -config "$CONFIG_FILE" -is-enabled; then
        staged_commit=$(printf '%s' "$staged_version" | sed -n 's/.*commit: \([^,]*\),.*/\1/p')
        if ! verify_live "$remote_ver" "$staged_commit" "$started_at"; then
            rollback_and_verify "Candidate fresh identity and successful inform were not verified"; rm -rf "$tmp_dir"; return 1
        fi
    else
        enabled_result=$?
        if [ "$enabled_result" -ne 3 ]; then
            rollback_and_verify "Installed configuration could not be validated"; rm -rf "$tmp_dir"; return 1
        fi
        live_pids=$(pidof unifi-bridge 2>/dev/null | tr -d ' \t\r\n')
        status_snapshot=$(cat "$STATUS_JS" 2>/dev/null)
        disabled_updated=$(printf '%s' "$status_snapshot" | sed -n 's/.*"status_updated_at":[[:space:]]*\([0-9][0-9]*\).*/\1/p' | head -n 1)
        if [ -n "$live_pids" ] || ! printf '%s' "$status_snapshot" | grep -Eq '"enabled"[[:space:]]*:[[:space:]]*false' ||
           ! printf '%s' "$status_snapshot" | grep -Eq '"running"[[:space:]]*:[[:space:]]*false' ||
           [ -z "$disabled_updated" ] || [ "$disabled_updated" -lt "$started_at" ] 2>/dev/null ||
           ! printf '%s' "$status_snapshot" | grep -Eq "\"version\"[[:space:]]*:[[:space:]]*\"$remote_ver\"" ||
           ! printf '%s' "$status_snapshot" | grep -Eq "\"commit\"[[:space:]]*:[[:space:]]*\"$staged_commit\""; then
            rollback_and_verify "Disabled installation did not publish matching disabled status"; rm -rf "$tmp_dir"; return 1
        fi
    fi

    printf 'ACCEPTED\n' > "$TXN_DIR/active.tmp" && mv -f "$TXN_DIR/active.tmp" "$TXN_DIR/active" && sync || {
        rollback_and_verify "Could not persist accepted transaction state"; rm -rf "$tmp_dir"; return 1
    }
    clear_transaction || {
        write_update_status "ERROR" "$branch" "$local_ver" "$remote_ver" "Candidate verified but transaction cleanup failed; inspect $TXN_DIR"
        rm -rf "$tmp_dir"; return 1
    }

    write_update_status "SUCCESS" "$branch" "$remote_ver" "$remote_ver" "Local addon updated to ${remote_ver} (${branch}); cluster rollout is reported separately"

    # Release every local lock before contacting nodes. The sync process takes
    # an immutable identity snapshot, then each receiver enters its own updater.
    release_activation_lock
    release_mutation_lock
    release_update_lock
    # Only an operator-originated Main update fans out. apply-exact is the
    # receiver-side primitive used by sync_nodes.sh; propagating from it would
    # create reciprocal updates between AiMesh peers.
    if should_propagate_update; then
        echo "[*] Propagating update to cluster nodes..."
        if [ -f "$INSTALL_DIR/sync_nodes.sh" ]; then
            UNIFI_BRIDGE_MUTATION_LOCK_OWNER= UNIFI_BRIDGE_START_LOCK_OWNER= \
                "$INSTALL_DIR/sync_nodes.sh" update "$branch" "$resolved_commit" "$request_id" >/tmp/unifi-bridge-sync.log 2>&1 &
        fi
    fi

    rm -rf "$tmp_dir"
    echo "====================================================="
    echo " Update completed successfully! Version: ${remote_ver}"
    echo "====================================================="
    return 0
}

cmd_status() {
    if [ -f "$UPDATE_JS" ]; then
        cat "$UPDATE_JS"
    else
        branch=$(get_branch)
        ver=$(get_local_version)
        echo "Current Version: $ver ($branch)"
    fi
}

should_propagate_update() {
    [ "${operation:-}" = "apply" ]
}

case "${1:-}" in
    check)
        acquire_update_lock || exit 1
        cmd_check "${2:-}" "${3:-}"
        ;;
    apply|update)
        acquire_update_lock || exit 1
        cmd_apply "${2:-}" "${3:-}"
        ;;
    apply-exact)
        acquire_update_lock || exit 1
        cmd_apply "${2:-}" "${4:-}" "${3:-}"
        ;;
    status)
        cmd_status
        ;;
    *)
        echo "Usage: $0 {check [branch]|apply [branch]|apply-exact [branch commit request_id]|status}"
        exit 1
        ;;
esac
