#!/bin/sh
# ==============================================================================
# UniFi AP Bridge - Apply Configuration Handler for Asuswrt-Merlin
# ==============================================================================

INSTALL_DIR="/jffs/addons/unifi-bridge"
CONF_DIR="$INSTALL_DIR/config"
CONF_FILE="$CONF_DIR/config.json"
SETTINGS_FILE="/jffs/addons/custom_settings.txt"
MUTATION_LOCK="/tmp/unifi-bridge-mutation.lock"
mutation_lock_owned=0

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
    trap 'release_mutation_lock' 0
    trap 'exit 130' INT
    trap 'exit 143' TERM
}
assert_no_update_journal() {
    [ ! -e "$INSTALL_DIR/.update-transaction" ] || {
        echo "Unfinished addon update journal requires recovery before mutation" >&2; return 1;
    }
}


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
        if [ -f "$INSTALL_DIR/sync_nodes.sh" ]; then
            "$INSTALL_DIR/sync_nodes.sh" sync >/tmp/unifi-bridge-sync.log 2>&1 &
        fi
        exit 0
        ;;
esac

# Parse and merge JSON using the daemon's dependency-free settings helper.
# Never reconstruct config with grep/sed: node lists, mappings and unknown fields
# must survive unrelated WebUI edits. Older binaries fail closed on this flag.
acquire_mutation_lock || exit 1
assert_no_update_journal || exit 1
mkdir -p "$CONF_DIR" || exit 1
BRIDGE_BIN="/tmp/unifi-bridge/unifi-bridge"
[ -x "$BRIDGE_BIN" ] || BRIDGE_BIN="$INSTALL_DIR/bin/unifi-bridge"
[ -x "$BRIDGE_BIN" ] || { echo "Bridge settings helper unavailable" >&2; exit 1; }
GW_IP=$(nvram get lan_gateway)
AMNG=$(nvram get amng_custom 2>/dev/null)
if [ -n "$AMNG" ]; then
    printf '%s' "$AMNG" | UNIFI_BRIDGE_MUTATION_LOCK_HELD=1 UNIFI_BRIDGE_MUTATION_LOCK_OWNER="$$" \
        "$BRIDGE_BIN" -config "$CONF_FILE" -apply-settings -gateway "$GW_IP" || exit 1
elif [ -f "$SETTINGS_FILE" ]; then
    UNIFI_BRIDGE_MUTATION_LOCK_HELD=1 UNIFI_BRIDGE_MUTATION_LOCK_OWNER="$$" \
        "$BRIDGE_BIN" -config "$CONF_FILE" -apply-settings -settings-format custom -gateway "$GW_IP" < "$SETTINGS_FILE" || exit 1
else
    echo "No submitted settings" >&2
    exit 1
fi

# start.sh enforces enabled=false for local, cluster and boot entry points.
UNIFI_BRIDGE_MUTATION_LOCK_OWNER="$$" "$INSTALL_DIR/start.sh" || exit 1
if [ -f "$INSTALL_DIR/sync_nodes.sh" ]; then
    UNIFI_BRIDGE_MUTATION_LOCK_OWNER= UNIFI_BRIDGE_START_LOCK_OWNER= \
        "$INSTALL_DIR/sync_nodes.sh" sync >/tmp/unifi-bridge-sync.log 2>&1 &
fi
