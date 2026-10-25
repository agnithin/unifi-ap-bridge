#!/bin/sh
INSTALL_DIR="/jffs/addons/unifi-bridge"
CONFIG_FILE="$INSTALL_DIR/config/config.json"
TXN_DIR="$INSTALL_DIR/.update-transaction"
START_LOCK="/tmp/unifi-bridge-start.lock"
recovering=0

# W2 rollback runs before ordinary startup. The journal lists every managed
# generation file and records whether it existed before activation. Restore
# start.sh last, then discard the journal only after all atomic restores pass.
recover_update() {
    recovering=0
    if [ ! -f "$TXN_DIR/active" ]; then
        # The updater creates and syncs the complete snapshot before publishing
        # this marker; an unmarked directory cannot have changed active files.
        [ ! -e "$TXN_DIR" ] || rm -rf "$TXN_DIR" || return 1
        return 0
    fi
    [ -f "$TXN_DIR/manifest" ] || { echo "Update recovery journal is incomplete" >&2; return 1; }
    journal_state=$(cat "$TXN_DIR/active" 2>/dev/null | tr -d ' \t\r\n')
    if [ "$journal_state" = "ACCEPTED" ]; then
        mv "$TXN_DIR" "$INSTALL_DIR/.update-complete-$$" || return 1
        sync
        rm -rf "$INSTALL_DIR/.update-complete-$$" || return 1
        return 0
    fi
    [ "$journal_state" = "ACTIVATING" ] || { echo "Unknown addon transaction state" >&2; return 1; }
    failed=0
    while IFS=' ' read -r state rel; do
        [ "$rel" = "start.sh" ] && continue
        case "$rel" in
            unifi-bridge.tar.gz|unifi-bridge.tar.gz.sha256|apply_config.sh|sync_nodes.sh|unifi-bridge.sh|update.sh|watchdog.sh|version.txt|webui/unifi-bridge.asp) ;;
            *) failed=1; break ;;
        esac
        dest="$INSTALL_DIR/$rel"
        mkdir -p "$(dirname "$dest")" || { failed=1; break; }
        if [ "$state" = present ]; then
            src="$TXN_DIR/snapshot/$rel"
            [ -f "$src" ] || { failed=1; break; }
            tmp="$dest.restore.$$"
            if ! cp -p "$src" "$tmp" || ! mv -f "$tmp" "$dest"; then
                rm -f "$tmp"; failed=1; break
            fi
        elif [ "$state" = absent ]; then
            rm -f "$dest" || { failed=1; break; }
        else
            failed=1; break
        fi
    done < "$TXN_DIR/manifest"
    if [ "$failed" -ne 0 ]; then
        echo "Update recovery failed; journal retained at $TXN_DIR" >&2
        return 1
    fi
    if ! grep -q '^present start.sh$' "$TXN_DIR/manifest"; then
        echo "Prior start.sh was absent; refusing to remove recovery entry point" >&2
        return 1
    fi
    sync
    recovery_arch=$(uname -m)
    case "$recovery_arch" in armv7l) recovery_member=unifi-bridge-armv7 ;; aarch64) recovery_member=unifi-bridge-arm64 ;; *) return 1 ;; esac
    recovery_dir=/tmp/unifi-bridge/recovery-check
    rm -rf "$recovery_dir"; mkdir -p "$recovery_dir" || return 1
    expected_sum=$(awk -v name=unifi-bridge.tar.gz 'NF==2 && $2==name {print $1}' "$TXN_DIR/snapshot/unifi-bridge.tar.gz.sha256")
    actual_sum=$(openssl dgst -sha256 "$TXN_DIR/snapshot/unifi-bridge.tar.gz" 2>/dev/null | awk '{print $NF}')
    [ "${#expected_sum}" -eq 64 ] && [ "$expected_sum" = "$actual_sum" ] || return 1
    tar -tzf "$TXN_DIR/snapshot/unifi-bridge.tar.gz" > "$recovery_dir/members" 2>/dev/null || return 1
    tar -tvzf "$TXN_DIR/snapshot/unifi-bridge.tar.gz" > "$recovery_dir/types" 2>/dev/null || return 1
    recovery_listing=$(LC_ALL=C sort "$recovery_dir/members")
    recovery_expected="apply_config.sh
bundle.json
install.sh
start.sh
sync_nodes.sh
${recovery_member}
unifi-bridge.asp
unifi-bridge.sh
update.sh
version.txt
watchdog.sh"
    [ "$recovery_listing" = "$recovery_expected" ] || return 1
    recovery_types=$(awk 'substr($0,1,1)!="-" || substr($0,6,1)=="w" || substr($0,9,1)=="w" || substr($0,4,1) ~ /[sS]/ || substr($0,7,1) ~ /[sS]/ || substr($0,10,1) ~ /[tT]/ { print "invalid"; exit } END { if (!NR) print "invalid" }' "$recovery_dir/types")
    [ -z "$recovery_types" ] || return 1
    tar -xzf "$TXN_DIR/snapshot/unifi-bridge.tar.gz" -C "$recovery_dir" >/dev/null 2>&1 || return 1
    [ -f "$recovery_dir/$recovery_member" ] || return 1
    recovery_identity=$("$recovery_dir/$recovery_member" -version 2>/dev/null) || return 1
    recovery_version=$(printf '%s\n' "$recovery_identity" | sed -n 's/^unifi-bridge \([^ ]*\).*/\1/p')
    recovery_commit=$(printf '%s\n' "$recovery_identity" | sed -n 's/.*commit: \([^,]*\),.*/\1/p')
    case "$recovery_commit" in *[!0-9a-f]*|'') return 1 ;; esac
    [ "${#recovery_commit}" -eq 40 ] && [ -n "$recovery_version" ] || return 1
    if "$recovery_dir/$recovery_member" -config "$CONFIG_FILE" -is-enabled; then
        recovery_enabled=1
    else
        enabled_rc=$?
        [ "$enabled_rc" -eq 3 ] || return 1
        recovery_enabled=0
    fi
    recovering=1
    return 0
}

finish_recovery() {
    [ "$recovering" -eq 1 ] || return 0
    tmp="$INSTALL_DIR/start.sh.restore.$$"
    cp -p "$TXN_DIR/snapshot/start.sh" "$tmp" && mv -f "$tmp" "$INSTALL_DIR/start.sh" || {
        rm -f "$tmp"; echo "Could not restore prior start.sh; recovery journal retained" >&2; return 1
    }
    sync
    mv "$TXN_DIR" "$INSTALL_DIR/.update-complete-$$" || return 1
    sync
    rm -rf "$INSTALL_DIR/.update-complete-$$" || return 1
    recovering=0
    return 0
}

MUTATION_LOCK="/tmp/unifi-bridge-mutation.lock"
mutation_lock_owned=0
start_lock_owned=0
release_start_locks() {
    if [ "$start_lock_owned" -eq 1 ] && [ "$(cat "$START_LOCK/pid" 2>/dev/null)" = "$$" ]; then
        rm -f "$START_LOCK/pid"; rmdir "$START_LOCK" 2>/dev/null || true
    fi
    if [ "$mutation_lock_owned" -eq 1 ] && [ "$(cat "$MUTATION_LOCK/pid" 2>/dev/null)" = "$$" ]; then
        rm -f "$MUTATION_LOCK/pid"; rmdir "$MUTATION_LOCK" 2>/dev/null || true
    fi
}
trap 'release_start_locks' 0
trap 'exit 130' INT
trap 'exit 143' TERM
# All persistent mutations follow mutation -> start. Inherited ownership is
# accepted only when the supplied owner token matches the lock record.
if [ -n "${UNIFI_BRIDGE_MUTATION_LOCK_OWNER:-}" ]; then
    [ "$(cat "$MUTATION_LOCK/pid" 2>/dev/null)" = "$UNIFI_BRIDGE_MUTATION_LOCK_OWNER" ] || {
        echo "Inherited mutation lock ownership is invalid" >&2; exit 1;
    }
else
    tries=0
    mutation_wait=${UNIFI_BRIDGE_MUTATION_WAIT_SECONDS:-30}
    while ! (umask 077; mkdir "$MUTATION_LOCK") 2>/dev/null; do
        [ "$tries" -lt "$mutation_wait" ] || { echo "Mutation lock busy; owner $(cat "$MUTATION_LOCK/pid" 2>/dev/null || echo unknown)" >&2; exit 1; }
        tries=$((tries + 1)); sleep 1
    done
    echo "$$" > "$MUTATION_LOCK/pid" || { rmdir "$MUTATION_LOCK" 2>/dev/null; exit 1; }
    mutation_lock_owned=1
    export UNIFI_BRIDGE_MUTATION_LOCK_OWNER="$$"
fi
if [ -n "${UNIFI_BRIDGE_START_LOCK_OWNER:-}" ]; then
    [ "$(cat "$START_LOCK/pid" 2>/dev/null)" = "$UNIFI_BRIDGE_START_LOCK_OWNER" ] || {
        echo "Inherited start lock ownership is invalid" >&2; exit 1;
    }
else
    if ! (umask 077; mkdir "$START_LOCK") 2>/dev/null; then
        echo "Another bridge start or activation owns $START_LOCK" >&2
        exit 1
    fi
    echo "$$" > "$START_LOCK/pid" || { rmdir "$START_LOCK" 2>/dev/null; exit 1; }
    start_lock_owned=1
fi

if [ "${1:-}" = "--recover-only" ]; then
    recover_update || exit 1
    exit 0
fi
for completed in "$INSTALL_DIR"/.update-complete-*; do
    [ -d "$completed" ] || continue
    rm -rf "$completed" || exit 1
done
if [ -z "${UNIFI_BRIDGE_START_LOCK_OWNER:-}" ]; then
    recover_update || exit 1
fi

# Always extract to a fresh RAM staging directory and select only this CPU's
# member. This replaces a candidate executable explicitly after rollback.
mkdir -p /tmp/unifi-bridge || exit 1
arch=$(uname -m)
case "$arch" in armv7l) member=unifi-bridge-armv7 ;; aarch64) member=unifi-bridge-arm64 ;; *) echo "Unsupported bridge architecture: $arch" >&2; exit 1 ;; esac
rm -rf /tmp/unifi-bridge/staged
mkdir -p /tmp/unifi-bridge/staged || exit 1
if [ -f "$INSTALL_DIR/unifi-bridge.tar.gz" ]; then
    if ! tar -xzf "$INSTALL_DIR/unifi-bridge.tar.gz" -C /tmp/unifi-bridge/staged || [ ! -f "/tmp/unifi-bridge/staged/$member" ]; then
        echo "Could not extract the installed bridge generation" >&2; exit 1
    fi
    cp -f "/tmp/unifi-bridge/staged/$member" /tmp/unifi-bridge/unifi-bridge.new || exit 1
elif [ -f "$INSTALL_DIR/bin/unifi-bridge" ]; then
    cp -fp "$INSTALL_DIR/bin/unifi-bridge" /tmp/unifi-bridge/unifi-bridge.new || exit 1
else
    echo "No installed bridge binary archive" >&2; exit 1
fi
chmod 755 /tmp/unifi-bridge/unifi-bridge.new || exit 1
mv -f /tmp/unifi-bridge/unifi-bridge.new /tmp/unifi-bridge/unifi-bridge || exit 1

# Keep the management page available even when telemetry collection is
# intentionally disabled.
SLOT_FILE="$INSTALL_DIR/.user_slot"
USER_SLOT="user3"
[ -f "$SLOT_FILE" ] && USER_SLOT=$(cat "$SLOT_FILE" 2>/dev/null | tr -d ' \t\r\n')
[ -z "$USER_SLOT" ] && USER_SLOT="user3"
mkdir -p /tmp/var/wwwext/unifi-bridge
if [ -f "$INSTALL_DIR/webui/unifi-bridge.asp" ]; then
    cp -fp "$INSTALL_DIR/webui/unifi-bridge.asp" "/tmp/var/wwwext/${USER_SLOT}.asp" || exit 1
    echo "UniFi AP Bridge" > "/tmp/var/wwwext/${USER_SLOT}.title"
fi

stop_bridge() {
    killall unifi-bridge 2>/dev/null || true
    tries=0
    while [ "$tries" -lt 10 ]; do
        remaining=$(pidof unifi-bridge 2>/dev/null | tr -d ' \t\r\n')
        [ -z "$remaining" ] && break
        tries=$((tries + 1)); sleep 1
    done
    remaining=$(pidof unifi-bridge 2>/dev/null | tr -d ' \t\r\n')
    [ -z "$remaining" ] || { echo "Bridge process did not stop: $remaining" >&2; return 1; }
    rm -f /tmp/var/wwwext/unifi-bridge/status.js
    return 0
}

# Guard boot, cluster and manual entry points after preparing the binary. The
# daemon publishes complete disabled metadata without accessing Wi-Fi/adoption.
if /tmp/unifi-bridge/unifi-bridge -config "$CONFIG_FILE" -is-enabled; then
    :
else
    enabled_result=$?
    [ "$enabled_result" -eq 3 ] || exit "$enabled_result"
    recovery_started_at=$(date +%s)
    stop_bridge || exit 1
    /tmp/unifi-bridge/unifi-bridge -config "$CONFIG_FILE" -once || exit $?
    if [ "$recovering" -eq 1 ]; then
        status_snapshot=$(cat /tmp/var/wwwext/unifi-bridge/status.js 2>/dev/null)
        disabled_updated=$(printf '%s' "$status_snapshot" | sed -n 's/.*"status_updated_at":[[:space:]]*\([0-9][0-9]*\).*/\1/p' | head -n 1)
        printf '%s' "$status_snapshot" | grep -Eq '"enabled"[[:space:]]*:[[:space:]]*false' || exit 1
        printf '%s' "$status_snapshot" | grep -Eq '"running"[[:space:]]*:[[:space:]]*false' || exit 1
        [ -n "$disabled_updated" ] && [ "$disabled_updated" -ge "$recovery_started_at" ] 2>/dev/null || exit 1
        printf '%s' "$status_snapshot" | grep -Eq "\"version\"[[:space:]]*:[[:space:]]*\"$recovery_version\"" || exit 1
        printf '%s' "$status_snapshot" | grep -Eq "\"commit\"[[:space:]]*:[[:space:]]*\"$recovery_commit\"" || exit 1
        finish_recovery || exit 1
    fi
    exit 0
fi

# WebUI mount
# Safely update /tmp/menuTree.js without clobbering other addons (scMerlin, MerVLAN)
if [ -f /www/require/modules/menuTree.js ]; then
    if [ ! -f /tmp/menuTree.js ]; then
        cp -fp /www/require/modules/menuTree.js /tmp/menuTree.js
    fi

    # Only insert if not already present
    if ! grep -q "${USER_SLOT}.asp" /tmp/menuTree.js; then
        sed -i '/tabName: "UniFi AP Bridge"/d' /tmp/menuTree.js
        sed -i "/${USER_SLOT}\.asp/d" /tmp/menuTree.js
        sed -i "/ur[0-9]\+\.asp/d" /tmp/menuTree.js

        awk -v slot="${USER_SLOT}.asp" '
        /index: "menu_Wireless"/ { in_w = 1 }
        in_w && /url: "NULL"/ {
            printf "{url: \"%s\", tabName: \"UniFi AP Bridge\"},\n", slot
            in_w = 0
        }
        { print }
        ' /tmp/menuTree.js > /tmp/menuTree.tmp && mv -f /tmp/menuTree.tmp /tmp/menuTree.js

        umount /www/require/modules/menuTree.js 2>/dev/null || true
        mount -o bind /tmp/menuTree.js /www/require/modules/menuTree.js 2>/dev/null || true
    elif ! mount | grep -q "/www/require/modules/menuTree.js"; then
        mount -o bind /tmp/menuTree.js /www/require/modules/menuTree.js 2>/dev/null || true
    fi
fi

# Arm Watchdog cron
if type cru >/dev/null 2>&1; then
    cru a unifi_bridge_watchdog "*/5 * * * * $INSTALL_DIR/watchdog.sh" 2>/dev/null || true
fi

stop_bridge || exit 1
recovery_started_at=$(date +%s)
nohup /tmp/unifi-bridge/unifi-bridge -config "$INSTALL_DIR/config/config.json" >/tmp/unifi-bridge.log 2>&1 &
if [ "$recovering" -eq 1 ]; then
    attempts=0
    while [ "$attempts" -lt 30 ]; do
        pid_list=$(pidof unifi-bridge 2>/dev/null | tr -s ' ' | sed 's/^ //;s/ $//')
        status_snapshot=$(cat /tmp/var/wwwext/unifi-bridge/status.js 2>/dev/null)
        status_pid=$(printf '%s' "$status_snapshot" | sed -n 's/.*"pid":[[:space:]]*\([0-9][0-9]*\).*/\1/p' | head -n 1)
        status_updated=$(printf '%s' "$status_snapshot" | sed -n 's/.*"status_updated_at":[[:space:]]*\([0-9][0-9]*\).*/\1/p' | head -n 1)
        successful_inform=$(printf '%s' "$status_snapshot" | sed -n 's/.*"last_successful_inform":[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)
        process_exe=$(readlink "/proc/$pid_list/exe" 2>/dev/null)
        if [ "$recovery_enabled" = 1 ] && [ -n "$pid_list" ] && ! printf '%s' "$pid_list" | grep -q ' ' &&
           [ "$pid_list" = "$status_pid" ] && [ "$status_updated" -ge "$recovery_started_at" ] 2>/dev/null &&
           [ "$process_exe" = "/tmp/unifi-bridge/unifi-bridge" ] &&
           printf '%s' "$status_snapshot" | grep -Fq "\"version\": \"$recovery_version\"" &&
           printf '%s' "$status_snapshot" | grep -Fq "\"commit\": \"$recovery_commit\"" &&
           printf '%s' "$status_snapshot" | grep -Fq '"running": true' &&
           printf '%s' "$status_snapshot" | grep -Fq '"last_status": "Active (' &&
           [ -n "$successful_inform" ]; then
            finish_recovery || exit 1
            break
        fi
        attempts=$((attempts + 1)); sleep 2
    done
    [ "$recovering" -eq 0 ] || { echo "Prior addon recovery remains unverified; journal retained" >&2; exit 1; }
fi
