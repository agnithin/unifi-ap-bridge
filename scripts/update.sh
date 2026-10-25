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
acquire_update_lock() {
    if ! (umask 077; mkdir "$UPDATE_LOCK") 2>/dev/null; then
        echo "Another bridge update/check owns $UPDATE_LOCK; retry after it completes. If interrupted, inspect service and lock ownership before recovery." >&2
        return 1
    fi
    if ! echo "$$" > "$UPDATE_LOCK/pid"; then
        rmdir "$UPDATE_LOCK" 2>/dev/null || true
        return 1
    fi
    trap 'release_update_lock' 0
    trap 'exit 130' INT
    trap 'exit 143' TERM
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

    # Resolve one immutable release generation. Dev uses a SHA-qualified
    # release tag; stable resolves the official latest tag before asset fetch.
    if [ "$branch" = "dev" ]; then
        resolved_commit=$(curl -fsSL --connect-timeout 8 --max-time 20 \
            -H 'Accept: application/vnd.github+json' \
            'https://api.github.com/repos/agnithin/unifi-ap-bridge/commits/dev' 2>/dev/null |
            awk -F'"' '/"sha"[[:space:]]*:/ { print $4; exit }')
        case "$resolved_commit" in *[!0-9a-f]*|'') resolved_commit="" ;; esac
        [ "${#resolved_commit}" -eq 40 ] || resolved_commit=""
        release_tag="dev-${resolved_commit}"
        expected_version=$(curl -fsSL --connect-timeout 8 --max-time 20 \
            "${REPO_RAW}/${resolved_commit}/version.txt" 2>/dev/null | tr -d ' \t\r\n')
    else
        release_tag=$(curl -fsSL --connect-timeout 8 --max-time 20 \
            -H 'Accept: application/vnd.github+json' \
            'https://api.github.com/repos/agnithin/unifi-ap-bridge/releases/latest' 2>/dev/null |
            awk -F'"' '/"tag_name"[[:space:]]*:/ { print $4; exit }')
        resolved_commit=$(curl -fsSL --connect-timeout 8 --max-time 20 \
            -H 'Accept: application/vnd.github+json' \
            "https://api.github.com/repos/agnithin/unifi-ap-bridge/commits/${release_tag}" 2>/dev/null |
            awk -F'"' '/"sha"[[:space:]]*:/ { print $4; exit }')
        expected_version=${release_tag#v}
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
    mkdir -p "$INSTALL_DIR/.ssh"
    chmod 755 "$INSTALL_DIR"
    chmod 700 "$INSTALL_DIR/.ssh"
    cp -f "$bundle_archive" "$INSTALL_DIR/unifi-bridge.tar.gz" || {
        write_update_status "ERROR" "$branch" "$local_ver" "$remote_ver" "Could not stage verified addon bundle"
        rm -rf "$tmp_dir"; return 1
    }

    # 3. Apply updated scripts & WebUI
    echo "[*] Installing updated scripts to ${INSTALL_DIR}..."
    for f in start.sh apply_config.sh sync_nodes.sh unifi-bridge.sh update.sh watchdog.sh; do
        if ! cp -fp "${tmp_dir}/staged/$f" "$INSTALL_DIR/$f" || ! chmod 755 "$INSTALL_DIR/$f"; then
            write_update_status "ERROR" "$branch" "$local_ver" "$remote_ver" "Script installation failed; inspect service status before retrying"
            rm -rf "$tmp_dir"; return 1
        fi
    done
    ln -sf "$INSTALL_DIR/unifi-bridge.sh" "/jffs/scripts/unifi-bridge"
    mkdir -p "$INSTALL_DIR/webui"
    slot="user3"
    [ -f "$INSTALL_DIR/.user_slot" ] && slot=$(cat "$INSTALL_DIR/.user_slot" 2>/dev/null | tr -d ' \t\r\n')
    [ -z "$slot" ] && slot="user3"
    if ! cp -fp "${tmp_dir}/staged/version.txt" "$INSTALL_DIR/version.txt" ||
       ! cp -fp "${tmp_dir}/staged/unifi-bridge.asp" "$INSTALL_DIR/webui/unifi-bridge.asp" ||
       ! cp -fp "${tmp_dir}/staged/unifi-bridge.asp" "/tmp/var/wwwext/${slot}.asp"; then
        write_update_status "ERROR" "$branch" "$local_ver" "$remote_ver" "UI installation failed; inspect service status before retrying"
        rm -rf "$tmp_dir"; return 1
    fi
    chmod 644 "$INSTALL_DIR/version.txt"

    # 4. Update branch in config.json
    if [ -f "$CONFIG_FILE" ]; then
        if grep -q '"branch"' "$CONFIG_FILE"; then
            sed -i "s/\"branch\":[[:space:]]*\"[^\"]*\"/\"branch\": \"${branch}\"/" "$CONFIG_FILE"
        else
            sed -i "s/}/,\n  \"branch\": \"${branch}\"\n}/" "$CONFIG_FILE"
        fi
    fi

    # 5. Restart local daemon cleanly
    echo "[*] Restarting UniFi AP Bridge daemon..."
    started_at=$(date +%s)
    if ! "$INSTALL_DIR/start.sh"; then
        write_update_status "ERROR" "$branch" "$local_ver" "$remote_ver" "Bridge startup failed; inspect service status before retrying"
        rm -rf "$tmp_dir"; return 1
    fi

    # A shell launch is not proof that the daemon is running the new binary or
    # exchanging informs. Disabled installs intentionally have no heartbeat.
    if "$tmp_dir/staged/unifi-bridge-${ARCH_SUFFIX}" -config "$CONFIG_FILE" -is-enabled; then
        attempts=0
        verified=0
        staged_commit=$(printf '%s' "$staged_version" | sed -n 's/.*commit: \([^,]*\),.*/\1/p')
        while [ "$attempts" -lt 30 ]; do
            live_pid=$(pidof unifi-bridge 2>/dev/null | awk '{print $1}')
            updated=$(grep '"status_updated_at":' "$STATUS_JS" 2>/dev/null | awk -F: '{print $2}' | tr -d ' ,\t\r\n')
            case "$live_pid:$updated" in *[!0-9:]*|:*|*:) ;;
                *) if [ "$updated" -ge "$started_at" ] &&
                      grep -Eq "\"pid\":[[:space:]]*${live_pid}[,[:space:]]" "$STATUS_JS" &&
                      grep -Fq "\"version\": \"${remote_ver}\"" "$STATUS_JS" &&
                      grep -Fq "\"commit\": \"${staged_commit}\"" "$STATUS_JS" &&
                      grep -Fq '"last_status": "Active (' "$STATUS_JS"; then
                    verified=1; break
                   fi ;;
            esac
            attempts=$((attempts + 1))
            sleep 2
        done
        if [ "$verified" -ne 1 ]; then
            write_update_status "ERROR" "$branch" "$local_ver" "$remote_ver" "New daemon heartbeat and build identity were not verified; inspect service status"
            rm -rf "$tmp_dir"; return 1
        fi
    else
        enabled_result=$?
        if [ "$enabled_result" -ne 3 ]; then
            write_update_status "ERROR" "$branch" "$local_ver" "$remote_ver" "Installed configuration could not be validated"
            rm -rf "$tmp_dir"; return 1
        fi
    fi

    write_update_status "SUCCESS" "$branch" "$remote_ver" "$remote_ver" "Local addon updated to ${remote_ver} (${branch}); cluster rollout is reported separately"

    echo "[*] Propagating update to cluster nodes..."
    if [ -f "$INSTALL_DIR/sync_nodes.sh" ]; then
        "$INSTALL_DIR/sync_nodes.sh" update "$branch" >/tmp/unifi-bridge-sync.log 2>&1 &
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

case "${1:-}" in
    check)
        acquire_update_lock || exit 1
        cmd_check "${2:-}" "${3:-}"
        ;;
    apply|update)
        acquire_update_lock || exit 1
        cmd_apply "${2:-}" "${3:-}"
        ;;
    status)
        cmd_status
        ;;
    *)
        echo "Usage: $0 {check [branch]|apply [branch]|status}"
        exit 1
        ;;
esac
