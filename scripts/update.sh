#!/bin/sh
# ==============================================================================
# UniFi AP Bridge - Update & Track Management Tool for Asuswrt-Merlin
# ==============================================================================

ADDON_NAME="unifi-bridge"
INSTALL_DIR="/jffs/addons/$ADDON_NAME"
# Protect existing cluster credentials as well as newly provisioned directories.
mkdir -p "$INSTALL_DIR/.ssh"
chmod 755 "$INSTALL_DIR"
chmod 700 "$INSTALL_DIR/.ssh"

CONFIG_FILE="$INSTALL_DIR/config/config.json"
VERSION_FILE="$INSTALL_DIR/version.txt"
STATUS_JS="/tmp/var/wwwext/$ADDON_NAME/status.js"
UPDATE_JS="/tmp/var/wwwext/$ADDON_NAME/update_status.js"
REPO_RAW="https://raw.githubusercontent.com/agnithin/unifi-ap-bridge"
REPO_RELEASES="https://github.com/agnithin/unifi-ap-bridge/releases"

mkdir -p "/tmp/var/wwwext/$ADDON_NAME"

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
    branch=$(get_branch "${1:-}")
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

    version_cmp "$clean_local" "$clean_remote"
    cmp_res=$?

    if [ "$cmp_res" -eq 0 ]; then
        echo "UniFi AP Bridge is up to date (version: ${raw_ver}, branch: ${branch})"
        write_update_status "UP_TO_DATE" "$branch" "$local_ver" "$raw_ver" "UniFi AP Bridge is up to date (${raw_ver})"
    elif [ "$cmp_res" -eq 1 ]; then
        echo "Installed version (${local_ver}) is newer than remote (${raw_ver}) on branch '${branch}'"
        write_update_status "UP_TO_DATE" "$branch" "$local_ver" "$raw_ver" "Installed version (${local_ver}) is up to date (newer than ${raw_ver})"
    else
        echo "New version available: ${raw_ver} (installed: ${local_ver})"
        write_update_status "UPDATE_AVAILABLE" "$branch" "$local_ver" "$raw_ver" "Update available: ${raw_ver}"
    fi
    return 0
}

cmd_apply() {
    branch=$(get_branch "${1:-}")
    local_ver=$(get_local_version)
    tmp_dir="/tmp/unifi-update-$$"

    echo "====================================================="
    echo " Starting update on track '${branch}'..."
    echo "====================================================="

    write_update_status "UPDATING" "$branch" "$local_ver" "-" "Downloading update files from track '${branch}'..."

    mkdir -p "$tmp_dir"
    rm -rf "${tmp_dir:?}"/*

    # Detect hardware architecture
    ARCH=$(uname -m)
    case "$ARCH" in
        armv7l) ARCH_SUFFIX="armv7" ;;
        aarch64) ARCH_SUFFIX="arm64" ;;
        *) ARCH_SUFFIX="armv7" ;;
    esac

    # 1. Download updated scripts, webui, and version.txt from raw GitHub
    echo "[*] Downloading scripts from branch '${branch}'..."
    failed=0
    cb=$(date +%s)
    for f in apply_config.sh sync_nodes.sh unifi-bridge.sh update.sh watchdog.sh; do
        if ! curl -fsSL --connect-timeout 8 --max-time 20 "${REPO_RAW}/${branch}/scripts/${f}?_=${cb}" -o "${tmp_dir}/${f}"; then
            echo "[!] Failed downloading scripts/${f}"
            failed=1
        fi
    done

    if ! curl -fsSL --connect-timeout 8 --max-time 20 "${REPO_RAW}/${branch}/webui/unifi-bridge.asp?_=${cb}" -o "${tmp_dir}/unifi-bridge.asp"; then
        echo "[!] Failed downloading webui/unifi-bridge.asp"
        failed=1
    fi

    if ! curl -fsSL --connect-timeout 8 --max-time 20 "${REPO_RAW}/${branch}/version.txt?_=${cb}" -o "${tmp_dir}/version.txt"; then
        echo "[!] Failed downloading version.txt"
        failed=1
    fi

    if [ "$failed" -eq 1 ]; then
        echo "Error: Failed to download repository files."
        write_update_status "ERROR" "$branch" "$local_ver" "-" "Download failed for repository scripts"
        rm -rf "$tmp_dir"
        return 1
    fi

    remote_ver=$(cat "${tmp_dir}/version.txt" 2>/dev/null | tr -d ' \t\r\n')
    [ -z "$remote_ver" ] && remote_ver="$branch"

    # 2. Download binary tarball if release exists
    bin_tarball="${tmp_dir}/unifi-bridge-${ARCH_SUFFIX}.tar.gz"
    echo "[*] Downloading binary for architecture ${ARCH_SUFFIX}..."

    bin_url=""
    if [ "$branch" = "main" ]; then
        bin_url="${REPO_RELEASES}/latest/download/unifi-bridge-${ARCH_SUFFIX}.tar.gz"
    else
        bin_url="${REPO_RELEASES}/download/dev/unifi-bridge-${ARCH_SUFFIX}.tar.gz"
    fi

    if curl -fsSL --connect-timeout 10 --max-time 60 "$bin_url" -o "$bin_tarball" 2>/dev/null; then
        echo "[*] Verified binary package download from releases."
        cp -f "$bin_tarball" "$INSTALL_DIR/unifi-bridge.tar.gz"
    elif curl -fsSL --connect-timeout 10 --max-time 60 "${REPO_RAW}/${branch}/bin/unifi-bridge-${ARCH_SUFFIX}.tar.gz?_=${cb}" -o "$bin_tarball" 2>/dev/null; then
        echo "[*] Verified binary package download from branch repository."
        cp -f "$bin_tarball" "$INSTALL_DIR/unifi-bridge.tar.gz"
    else
        echo "[!] Note: Binary package not found at ${bin_url}; keeping existing binary and updating scripts/WebUI."
    fi

    # 3. Apply updated scripts & WebUI
    echo "[*] Installing updated scripts to ${INSTALL_DIR}..."
    cp -fp "${tmp_dir}/apply_config.sh" "$INSTALL_DIR/apply_config.sh" && chmod 755 "$INSTALL_DIR/apply_config.sh"
    cp -fp "${tmp_dir}/sync_nodes.sh" "$INSTALL_DIR/sync_nodes.sh" && chmod 755 "$INSTALL_DIR/sync_nodes.sh"
    cp -fp "${tmp_dir}/update.sh" "$INSTALL_DIR/update.sh" && chmod 755 "$INSTALL_DIR/update.sh"
    cp -fp "${tmp_dir}/watchdog.sh" "$INSTALL_DIR/watchdog.sh" && chmod 755 "$INSTALL_DIR/watchdog.sh"
    cp -fp "${tmp_dir}/version.txt" "$INSTALL_DIR/version.txt" && chmod 644 "$INSTALL_DIR/version.txt"

    # CLI tool
    cp -fp "${tmp_dir}/unifi-bridge.sh" "$INSTALL_DIR/unifi-bridge.sh" && chmod 755 "$INSTALL_DIR/unifi-bridge.sh"
    ln -sf "$INSTALL_DIR/unifi-bridge.sh" "/jffs/scripts/unifi-bridge"

    # WebUI
    mkdir -p "$INSTALL_DIR/webui"
    cp -fp "${tmp_dir}/unifi-bridge.asp" "$INSTALL_DIR/webui/unifi-bridge.asp"
    slot="user3"
    [ -f "$INSTALL_DIR/.user_slot" ] && slot=$(cat "$INSTALL_DIR/.user_slot" 2>/dev/null | tr -d ' \t\r\n')
    [ -z "$slot" ] && slot="user3"
    cp -fp "${tmp_dir}/unifi-bridge.asp" "/tmp/var/wwwext/${slot}.asp"

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
    "$INSTALL_DIR/start.sh"

    write_update_status "SUCCESS" "$branch" "$remote_ver" "$remote_ver" "Successfully updated to ${remote_ver} (${branch})"
    (sleep 5 && write_update_status "UP_TO_DATE" "$branch" "$remote_ver" "$remote_ver" "UniFi AP Bridge is up to date (${remote_ver})") >/dev/null 2>&1 &

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
        cmd_check "${2:-}"
        ;;
    apply|update)
        cmd_apply "${2:-}"
        ;;
    status)
        cmd_status
        ;;
    *)
        echo "Usage: $0 {check [branch]|apply [branch]|status}"
        exit 1
        ;;
esac
