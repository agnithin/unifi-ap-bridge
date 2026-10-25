#!/bin/sh
# ==============================================================================
# UniFi AP Bridge - Health Check & Bounded Recovery Watchdog
# ==============================================================================

ADDON_NAME="unifi-bridge"
INSTALL_DIR="/jffs/addons/$ADDON_NAME"
CONFIG_FILE="$INSTALL_DIR/config/config.json"
WATCHDOG_LOG="/tmp/unifi-bridge-watchdog.log"
WATCHDOG_LOCK="/tmp/unifi-bridge-watchdog.lock"
WATCHDOG_STATE="/tmp/unifi-bridge-watchdog-state"
WATCHDOG_OBSERVED="/tmp/unifi-bridge-watchdog-observed"
STATUS_FILE="/tmp/var/wwwext/unifi-bridge/status.js"

if ! mkdir "$WATCHDOG_LOCK" 2>/dev/null; then
    owner=$(cat "$WATCHDOG_LOCK/pid" 2>/dev/null || true)
    case "$owner" in
        ''|*[!0-9]*) exit 0 ;;
    esac
    if /bin/kill -0 "$owner" 2>/dev/null; then exit 0; fi
    stale_lock="$WATCHDOG_LOCK.stale.$$"
    mv "$WATCHDOG_LOCK" "$stale_lock" 2>/dev/null || exit 0
    rm -rf "$stale_lock"
    mkdir "$WATCHDOG_LOCK" 2>/dev/null || exit 0
fi
echo "$$" > "$WATCHDOG_LOCK/pid"
trap 'rm -rf "$WATCHDOG_LOCK"' EXIT
trap 'exit 1' HUP INT TERM

# Startup, settings and activation own lifecycle changes while their locks or
# transaction journal are present. The watchdog never enters those windows.
MUTATION_LOCK="/tmp/unifi-bridge-mutation.lock"
mutation_lock_owned=0
release_mutation_lock() {
    if [ "$mutation_lock_owned" -eq 1 ] && [ "$(cat "$MUTATION_LOCK/pid" 2>/dev/null)" = "$$" ]; then
        rm -f "$MUTATION_LOCK/pid"; rmdir "$MUTATION_LOCK" 2>/dev/null || true
    fi
}
trap 'release_mutation_lock; rm -rf "$WATCHDOG_LOCK"' EXIT

recovery_blocked() {
    [ -d "/tmp/unifi-bridge-start.lock" ] && return 0
    [ -f "$INSTALL_DIR/.update-transaction/active" ] && return 0
    if [ -d "$MUTATION_LOCK" ]; then
        owner=$(cat "$MUTATION_LOCK/pid" 2>/dev/null || true)
        [ "$mutation_lock_owned" -eq 1 ] && [ "$owner" = "$$" ] && return 1
        return 0
    fi
    return 1
}

acquire_recovery_lock() {
    if ! mkdir "$MUTATION_LOCK" 2>/dev/null; then
        owner=$(cat "$MUTATION_LOCK/pid" 2>/dev/null || true)
        case "$owner" in ''|*[!0-9]*) return 1 ;; esac
        /bin/kill -0 "$owner" 2>/dev/null && return 1
        stale_lock="$MUTATION_LOCK.stale.$$"
        mv "$MUTATION_LOCK" "$stale_lock" 2>/dev/null || return 1
        rm -rf "$stale_lock"
        mkdir "$MUTATION_LOCK" 2>/dev/null || return 1
    fi
    echo "$$" > "$MUTATION_LOCK/pid" || { rmdir "$MUTATION_LOCK" 2>/dev/null || true; return 1; }
    mutation_lock_owned=1
    recovery_blocked && { release_mutation_lock; mutation_lock_owned=0; return 1; }
    export UNIFI_BRIDGE_MUTATION_LOCK_OWNER="$$"
    return 0
}

recovery_blocked && exit 0

if [ -f "$WATCHDOG_LOG" ] && [ "$(wc -c < "$WATCHDOG_LOG")" -gt 1048576 ]; then : > "$WATCHDOG_LOG"; fi
[ -f "$CONFIG_FILE" ] || exit 0

log_watchdog() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] Watchdog: $*" >> "$WATCHDOG_LOG"
}

read_field() {
    sed -n "s/.*\"$1\"[[:space:]]*:[[:space:]]*\([0-9][0-9]*\).*/\1/p" "$STATUS_FILE" 2>/dev/null | head -n 1
}

read_bool() {
    sed -n "s/.*\"$1\"[[:space:]]*:[[:space:]]*\([[:alpha:]]*\).*/\1/p" "$STATUS_FILE" 2>/dev/null | head -n 1
}

process_start() {
    [ -r "/proc/$1/stat" ] || return 1
    sed 's/^[^)]*) //' "/proc/$1/stat" 2>/dev/null | awk '{print $20}'
}

PID_LIST=$(pidof unifi-bridge 2>/dev/null || true)
PID_COUNT=$(printf '%s\n' "$PID_LIST" | awk '{print NF}')
[ -n "$PID_LIST" ] || PID_COUNT=0
case "$PID_COUNT" in
    0) PID="" ;;
    1) PID=$PID_LIST ;;
    *) log_watchdog "found multiple bridge processes ($PID_LIST); refusing recovery"; exit 1 ;;
esac
if [ -n "$PID" ]; then
    case "$PID" in *[!0-9]*) log_watchdog "pidof returned a malformed process ID; refusing recovery"; exit 1 ;; esac
fi

# Do not restart a disabled addon. The helper parses root configuration state.
if [ -x /tmp/unifi-bridge/unifi-bridge ]; then
    if /tmp/unifi-bridge/unifi-bridge -config "$CONFIG_FILE" -is-enabled; then
        :
    else
        enabled_result=$?
        [ "$enabled_result" -eq 3 ] && exit 0
        log_watchdog "configuration could not be validated; refusing recovery"
        exit 1
    fi
fi

NOW=$(date +%s)
case "$NOW" in *[!0-9]*|'') log_watchdog "invalid system clock; refusing recovery"; exit 1 ;; esac
INTERVAL=$(sed -n 's/.*"interval_sec"[[:space:]]*:[[:space:]]*\([0-9][0-9]*\).*/\1/p' "$STATUS_FILE" 2>/dev/null | head -n 1)
case "$INTERVAL" in *[!0-9]*|'') INTERVAL=10 ;; esac
STALE_AFTER=$((3 * INTERVAL + 53))
[ "$STALE_AFTER" -lt 180 ] && STALE_AFTER=180

STATUS_PID=$(read_field pid)
SAMPLE_UPDATED=$(read_field sample_updated_at)
SAMPLE_FRESH=$(read_bool sample_fresh)

# A fresh sample after a restart proves forward progress and clears accumulated
# restart backoff. Controller errors still advance status_updated_at.
set -- 0 0 0
[ -f "$WATCHDOG_STATE" ] && set -- $(cat "$WATCHDOG_STATE" 2>/dev/null)
FAILURES=${1:-0}; NEXT_ALLOWED=${2:-0}; LAST_PROGRESS=${3:-0}
case "$FAILURES:$NEXT_ALLOWED:$LAST_PROGRESS" in *[!0-9:]* ) FAILURES=0; NEXT_ALLOWED=0; LAST_PROGRESS=0 ;; esac
if [ -n "$PID" ] && [ "$STATUS_PID" = "$PID" ] && [ "$SAMPLE_FRESH" = true ] && [ -n "$SAMPLE_UPDATED" ] && [ "$SAMPLE_UPDATED" -gt "$LAST_PROGRESS" ] 2>/dev/null; then
    FAILURES=0; NEXT_ALLOWED=0; LAST_PROGRESS=$SAMPLE_UPDATED
    printf '%s %s %s\n' "$FAILURES" "$NEXT_ALLOWED" "$LAST_PROGRESS" > "$WATCHDOG_STATE.tmp" && mv -f "$WATCHDOG_STATE.tmp" "$WATCHDOG_STATE"
fi

restart_bridge() {
    [ "$NOW" -ge "$NEXT_ALLOWED" ] || { log_watchdog "restart backoff active until $NEXT_ALLOWED"; return 0; }
    if [ "$mutation_lock_owned" -ne 1 ]; then
        acquire_recovery_lock || { log_watchdog "restart deferred because lifecycle mutation is active"; return 0; }
    fi
    recovery_blocked && { log_watchdog "restart deferred because lifecycle changed"; return 0; }
    log_watchdog "starting bridge through tracked start.sh"
    UNIFI_BRIDGE_MUTATION_LOCK_OWNER="$$" "$INSTALL_DIR/start.sh" >> "$WATCHDOG_LOG" 2>&1
    start_result=$?
    FAILURES=$((FAILURES + 1))
    [ "$FAILURES" -gt 5 ] && FAILURES=5
    DELAY=$((300 << (FAILURES - 1)))
    [ "$DELAY" -gt 3600 ] && DELAY=3600
    NEXT_ALLOWED=$((NOW + DELAY))
    printf '%s %s %s\n' "$FAILURES" "$NEXT_ALLOWED" "$LAST_PROGRESS" > "$WATCHDOG_STATE.tmp" && mv -f "$WATCHDOG_STATE.tmp" "$WATCHDOG_STATE"
    if [ "$start_result" -eq 0 ]; then log_watchdog "start.sh returned success; next retry is rate-limited until $NEXT_ALLOWED"; else log_watchdog "start.sh failed ($start_result); next retry is rate-limited until $NEXT_ALLOWED"; fi
    return 0
}

if [ -z "$PID" ]; then
    restart_bridge
else
    START_ID=$(process_start "$PID" 2>/dev/null || true)
    if [ -z "$START_ID" ]; then log_watchdog "cannot identify process start time for PID $PID; refusing recovery"; exit 1; fi
    HEALTH_MATCH=false
    if [ "$STATUS_PID" = "$PID" ] && [ "$SAMPLE_FRESH" = true ] && [ -n "$SAMPLE_UPDATED" ]; then
        AGE=$((NOW - SAMPLE_UPDATED))
        if [ "$AGE" -ge 0 ] && [ "$AGE" -le "$STALE_AFTER" ]; then HEALTH_MATCH=true; fi
    fi
    if [ "$HEALTH_MATCH" = true ]; then
        rm -f "$WATCHDOG_OBSERVED"
    else
        set -- "" "" 0
        [ -f "$WATCHDOG_OBSERVED" ] && set -- $(cat "$WATCHDOG_OBSERVED" 2>/dev/null)
        SEEN_PID=${1:-}; SEEN_START=${2:-}; SEEN_AT=${3:-0}
        if [ "$SEEN_PID" != "$PID" ] || [ "$SEEN_START" != "$START_ID" ] || [ -z "$SEEN_AT" ]; then
            printf '%s %s %s\n' "$PID" "$START_ID" "$NOW" > "$WATCHDOG_OBSERVED.tmp" && mv -f "$WATCHDOG_OBSERVED.tmp" "$WATCHDOG_OBSERVED"
            log_watchdog "health record missing or stale for PID $PID; starting observation grace"
            exit 0
        fi
        OBSERVED_AGE=$((NOW - SEEN_AT))
        [ "$OBSERVED_AGE" -ge "$STALE_AFTER" ] || exit 0
        [ "$NOW" -ge "$NEXT_ALLOWED" ] || { log_watchdog "stale process recovery held by backoff until $NEXT_ALLOWED"; exit 0; }

        # Serialize with settings/update/start before rechecking lifecycle state
        # and PID identity. Keep ownership through restart so start.sh inherits it.
        acquire_recovery_lock || { log_watchdog "stale recovery deferred because lifecycle mutation is active"; exit 0; }
        recovery_blocked && { log_watchdog "stale recovery deferred because lifecycle changed"; exit 0; }
        [ "$(process_start "$PID" 2>/dev/null || true)" = "$START_ID" ] || { log_watchdog "PID $PID changed identity; refusing recovery"; exit 1; }
        STATUS_PID=$(read_field pid)
        SAMPLE_UPDATED=$(read_field sample_updated_at)
        SAMPLE_FRESH=$(read_bool sample_fresh)
        NOW=$(date +%s)
        if [ "$STATUS_PID" = "$PID" ] && [ "$SAMPLE_FRESH" = true ] && [ -n "$SAMPLE_UPDATED" ]; then
            AGE=$((NOW - SAMPLE_UPDATED))
            if [ "$AGE" -ge 0 ] && [ "$AGE" -le "$STALE_AFTER" ]; then
                rm -f "$WATCHDOG_OBSERVED"
                log_watchdog "sample progress recovered for PID $PID; canceling stale recovery"
                exit 0
            fi
        fi
        log_watchdog "PID $PID made no health progress for ${OBSERVED_AGE}s; stopping that bridge process"
        /bin/kill -TERM "$PID" 2>/dev/null || true
        tries=0
        while [ "$tries" -lt 10 ] && /bin/kill -0 "$PID" 2>/dev/null; do
            [ "$(process_start "$PID" 2>/dev/null || true)" = "$START_ID" ] || break
            sleep 1; tries=$((tries + 1))
        done
        if /bin/kill -0 "$PID" 2>/dev/null && [ "$(process_start "$PID" 2>/dev/null || true)" = "$START_ID" ]; then
            recovery_blocked && { log_watchdog "force termination deferred because lifecycle changed"; exit 1; }
            [ "$(process_start "$PID" 2>/dev/null || true)" = "$START_ID" ] || exit 1
            STATUS_PID=$(read_field pid)
            SAMPLE_UPDATED=$(read_field sample_updated_at)
            SAMPLE_FRESH=$(read_bool sample_fresh)
            NOW=$(date +%s)
            if [ "$STATUS_PID" = "$PID" ] && [ "$SAMPLE_FRESH" = true ] && [ -n "$SAMPLE_UPDATED" ]; then
                AGE=$((NOW - SAMPLE_UPDATED))
                if [ "$AGE" -ge 0 ] && [ "$AGE" -le "$STALE_AFTER" ]; then
                    log_watchdog "sample progress recovered during termination; refusing force kill"
                    exit 1
                fi
            fi
            /bin/kill -KILL "$PID" 2>/dev/null || true
        fi
        if /bin/kill -0 "$PID" 2>/dev/null && [ "$(process_start "$PID" 2>/dev/null || true)" = "$START_ID" ]; then
            log_watchdog "PID $PID survived termination; refusing to start a duplicate"
            exit 1
        fi
        rm -f "$WATCHDOG_OBSERVED"
        restart_bridge
    fi
fi

# Refresh cluster status on Main AP. This is independent of bridge recovery.
if [ -f "$INSTALL_DIR/sync_nodes.sh" ]; then
    "$INSTALL_DIR/sync_nodes.sh" status >/dev/null 2>&1 &
fi
