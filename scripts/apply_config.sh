#!/bin/sh
# ==============================================================================
# UniFi AP Bridge - Apply Configuration Handler for Asuswrt-Merlin
# ==============================================================================

INSTALL_DIR="/jffs/addons/unifi-bridge"
CONF_DIR="$INSTALL_DIR/config"
CONF_FILE="$CONF_DIR/config.json"
SETTINGS_FILE="/jffs/addons/custom_settings.txt"
MUTATION_LOCK="/tmp/unifi-bridge-mutation.lock"
ACTION_STATUS_DIR="/tmp/var/wwwext/unifi-bridge"
mutation_lock_owned=0
REQUEST_ID=""
action_status_started=0
action_status_done=0
settings_saved=0

release_mutation_lock() {
    if [ "$mutation_lock_owned" -eq 1 ] && [ "$(cat "$MUTATION_LOCK/pid" 2>/dev/null)" = "$$" ]; then
        rm -f "$MUTATION_LOCK/pid"; rmdir "$MUTATION_LOCK" 2>/dev/null || true
    fi
}
acquire_mutation_lock() {
    tries=0
    mutation_wait=${UNIFI_BRIDGE_MUTATION_WAIT_SECONDS:-30}
    while ! (umask 077; mkdir "$MUTATION_LOCK") 2>/dev/null; do
        [ "$tries" -lt "$mutation_wait" ] || { echo "Mutation lock busy; owner $(cat "$MUTATION_LOCK/pid" 2>/dev/null || echo unknown)" >&2; return 1; }
        tries=$((tries + 1)); sleep 1
    done
    echo "$$" > "$MUTATION_LOCK/pid" || { rmdir "$MUTATION_LOCK" 2>/dev/null; return 1; }
    mutation_lock_owned=1
    export UNIFI_BRIDGE_MUTATION_LOCK_OWNER="$$"
}
assert_no_update_journal() {
    [ ! -e "$INSTALL_DIR/.update-transaction" ] || {
        echo "Unfinished addon update journal requires recovery before mutation" >&2; return 1;
    }
}
write_action_status() {
    [ -n "$REQUEST_ID" ] || return 0
    status="$1"; message="$2"
    mkdir -p "$ACTION_STATUS_DIR" || return 1
    action_status_file="$ACTION_STATUS_DIR/action_status_${REQUEST_ID}.js"
    tmp="$action_status_file.tmp.$$"
    completed_at=$(date +%s)
    printf 'window.unifiBridgeAction = {"request_id":"%s","operation":"settings","status":"%s","completed_at":%s,"message":"%s"};\n' \
        "$REQUEST_ID" "$status" "$completed_at" "$message" > "$tmp" && chmod 644 "$tmp" && mv -f "$tmp" "$action_status_file" || return 1
    ls -1t "$ACTION_STATUS_DIR"/action_status_*.js 2>/dev/null | sed -n '21,$p' | while IFS= read -r old; do rm -f "$old"; done
}
cleanup_apply_config() {
    release_mutation_lock
    if [ "$action_status_started" -eq 1 ] && [ "$action_status_done" -eq 0 ]; then
        if [ "$settings_saved" -eq 1 ]; then
            write_action_status START_FAILED_AFTER_SAVE "Settings saved, but service startup did not complete"
        else
            write_action_status REJECTED "Settings were not changed; submission was rejected or superseded"
        fi
    fi
}
trap 'cleanup_apply_config' 0
trap 'exit 130' INT
trap 'exit 143' TERM


# POSIX parsing works with both BSD and BusyBox tools and legacy action names.
parse_update_action() {
    action_name="$1"
    action_suffix=${2##*"$action_name"_}
    BRANCH=${action_suffix%%_*}
    REQUEST_ID=${action_suffix#*_}
    [ "$REQUEST_ID" = "$action_suffix" ] && REQUEST_ID=""
    case "$BRANCH" in main|dev) ;; *) BRANCH="" ;; esac
    case "$REQUEST_ID" in ''|*[!0-9]*) REQUEST_ID="" ;; esac
}

# Actions triggered via service-event
case "${2:-}" in
    *check_update*)
        parse_update_action check_update "${2:-}"

        if [ -z "$BRANCH" ]; then
            AMNG=$(nvram get amng_custom 2>/dev/null)
            [ -n "$AMNG" ] && BRANCH=$(echo "$AMNG" | sed -n 's/.*"branch"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')
        fi
        if [ -z "$BRANCH" ] && [ -f "$CONF_FILE" ]; then
            BRANCH=$(grep '"branch"' "$CONF_FILE" 2>/dev/null | head -n 1 | awk -F'"' '{print $4}')
        fi
        if [ -z "$BRANCH" ] && [ -f "$SETTINGS_FILE" ]; then
            val=$(grep -E "^unifibridge_branch[[:space:]]+" "$SETTINGS_FILE" | tail -n 1 | awk '{print $2}')
            [ -n "$val" ] && BRANCH="$val"
        fi
        [ -z "$BRANCH" ] && BRANCH="main"

        # Checking another track must not persist a release-track change.

        if [ -f "$INSTALL_DIR/update.sh" ]; then
            "$INSTALL_DIR/update.sh" check "$BRANCH" "$REQUEST_ID" >/tmp/unifi-bridge-update.log 2>&1 &
        fi
        exit 0
        ;;
    *do_update*)
        parse_update_action do_update "${2:-}"

        if [ -z "$BRANCH" ]; then
            AMNG=$(nvram get amng_custom 2>/dev/null)
            [ -n "$AMNG" ] && BRANCH=$(echo "$AMNG" | sed -n 's/.*"branch"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')
        fi
        if [ -z "$BRANCH" ] && [ -f "$CONF_FILE" ]; then
            BRANCH=$(grep '"branch"' "$CONF_FILE" 2>/dev/null | head -n 1 | awk -F'"' '{print $4}')
        fi
        if [ -z "$BRANCH" ] && [ -f "$SETTINGS_FILE" ]; then
            val=$(grep -E "^unifibridge_branch[[:space:]]+" "$SETTINGS_FILE" | tail -n 1 | awk '{print $2}')
            [ -n "$val" ] && BRANCH="$val"
        fi
        [ -z "$BRANCH" ] && BRANCH="main"

        acquire_mutation_lock || exit 1
        assert_no_update_journal || exit 1
        if [ -f "$SETTINGS_FILE" ]; then
            tmp_settings="${SETTINGS_FILE}.tmp.$$"
            { grep -v "^unifibridge_branch" "$SETTINGS_FILE" 2>/dev/null || true; } > "$tmp_settings" || exit 1
            echo "unifibridge_branch $BRANCH" >> "$tmp_settings" || exit 1
            chmod 600 "$tmp_settings" && mv -f "$tmp_settings" "$SETTINGS_FILE" || exit 1
        fi
        release_mutation_lock
        mutation_lock_owned=0
        UNIFI_BRIDGE_MUTATION_LOCK_OWNER=

        if [ -f "$INSTALL_DIR/update.sh" ]; then
            "$INSTALL_DIR/update.sh" apply "$BRANCH" "$REQUEST_ID" >/tmp/unifi-bridge-update.log 2>&1 &
        fi
        exit 0
        ;;
    *sync*)
        case "${2:-}" in
            *_sync_*)
                sync_suffix=${2##*_sync_}
                REQUEST_ID=${sync_suffix%%_*}
                case "$REQUEST_ID" in ''|*[!0-9]*) echo "Invalid sync request ID" >&2; exit 1 ;; esac
                [ "${#REQUEST_ID}" -le 24 ] || { echo "Sync request ID exceeds limit" >&2; exit 1; }
                ;;
            *) REQUEST_ID="" ;;
        esac
        if [ -f "$INSTALL_DIR/sync_nodes.sh" ]; then
            "$INSTALL_DIR/sync_nodes.sh" sync "" "" "$REQUEST_ID" >/tmp/unifi-bridge-sync.log 2>&1 &
        fi
        exit 0
        ;;
esac

# The action suffix and root JSON request ID are checked by the Go settings
# helper. Capture the shared NVRAM payload before waiting on any lock so a later
# browser POST cannot silently replace the payload for this dispatched action.
case "${2:-}" in
    *_apply_*)
        REQUEST_ID=${2##*_apply_}
        case "$REQUEST_ID" in ''|*[!0-9]*) echo "Invalid settings request ID" >&2; exit 1 ;; esac
        [ "${#REQUEST_ID}" -le 24 ] || { echo "Settings request ID exceeds limit" >&2; exit 1; }
        ;;
    *) REQUEST_ID="" ;;
esac
if [ -n "$REQUEST_ID" ]; then
    action_status_started=1
    write_action_status RUNNING "Validating settings submission"
fi
AMNG=$(nvram get amng_custom 2>/dev/null)

# Parse and merge JSON using the daemon's dependency-free settings helper.
# Never reconstruct config with grep/sed: node lists, mappings and unknown fields
# must survive unrelated WebUI edits. Older binaries fail closed on this flag.
acquire_mutation_lock || exit 1
assert_no_update_journal || exit 1
BRIDGE_BIN="/tmp/unifi-bridge/unifi-bridge"
[ -x "$BRIDGE_BIN" ] || BRIDGE_BIN="$INSTALL_DIR/bin/unifi-bridge"
[ -x "$BRIDGE_BIN" ] || { echo "Bridge settings helper unavailable" >&2; exit 1; }
GW_IP=$(nvram get lan_gateway)
if [ -n "$REQUEST_ID" ] && [ -z "$AMNG" ]; then
    echo "ID-bound settings request has no captured JSON payload" >&2
    exit 1
fi
if [ -n "$AMNG" ]; then
    if [ -n "$REQUEST_ID" ]; then
        SETTINGS_RESULT=$(printf '%s' "$AMNG" | UNIFI_BRIDGE_MUTATION_LOCK_HELD=1 UNIFI_BRIDGE_MUTATION_LOCK_OWNER="$$" \
            "$BRIDGE_BIN" -config "$CONF_FILE" -apply-settings -gateway "$GW_IP" -request-id "$REQUEST_ID") || exit 1
    else
        SETTINGS_RESULT=$(printf '%s' "$AMNG" | UNIFI_BRIDGE_MUTATION_LOCK_HELD=1 UNIFI_BRIDGE_MUTATION_LOCK_OWNER="$$" \
            "$BRIDGE_BIN" -config "$CONF_FILE" -apply-settings -gateway "$GW_IP") || exit 1
    fi
elif [ -f "$SETTINGS_FILE" ]; then
    SETTINGS_RESULT=$(UNIFI_BRIDGE_MUTATION_LOCK_HELD=1 UNIFI_BRIDGE_MUTATION_LOCK_OWNER="$$" \
        "$BRIDGE_BIN" -config "$CONF_FILE" -apply-settings -settings-format custom -gateway "$GW_IP" < "$SETTINGS_FILE") || exit 1
else
    echo "No submitted settings" >&2
    exit 1
fi
# A semantic no-op is complete at the configuration layer. Do not restart
# this AP or dispatch a cluster sync for an unchanged save.
if [ "$SETTINGS_RESULT" = "SETTINGS_CHANGED=0" ]; then
    if [ "$action_status_started" -eq 1 ]; then
        write_action_status SAVED_NOOP "Settings already match; no service or node changes were needed"
        action_status_done=1
    fi
    exit 0
fi
settings_saved=1

# start.sh enforces enabled=false for local, cluster and boot entry points.
if ! UNIFI_BRIDGE_MUTATION_LOCK_OWNER="$$" "$INSTALL_DIR/start.sh"; then
    if [ "$action_status_started" -eq 1 ]; then
        write_action_status START_FAILED_AFTER_SAVE "Settings saved, but service startup failed"
        action_status_done=1
    fi
    exit 1
fi
if [ "$action_status_started" -eq 1 ]; then
    write_action_status SAVED "Settings saved on this AP; runtime and per-node outcomes remain unverified"
    action_status_done=1
fi
if [ -f "$INSTALL_DIR/sync_nodes.sh" ]; then
    UNIFI_BRIDGE_MUTATION_LOCK_OWNER= UNIFI_BRIDGE_START_LOCK_OWNER= \
        "$INSTALL_DIR/sync_nodes.sh" sync "" "" "$REQUEST_ID" >/tmp/unifi-bridge-sync.log 2>&1 &
fi
