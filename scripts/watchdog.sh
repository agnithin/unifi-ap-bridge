#!/bin/sh
# ==============================================================================
# UniFi AP Bridge - Health Check & Automatic Recovery Watchdog
# ==============================================================================

ADDON_NAME="unifi-bridge"
INSTALL_DIR="/jffs/addons/$ADDON_NAME"
CONFIG_FILE="$INSTALL_DIR/config/config.json"
WATCHDOG_LOG="/tmp/unifi-bridge-watchdog.log"

# Bound the RAM log even if repeated startup failures occur.
if [ -f "$WATCHDOG_LOG" ] && [ "$(wc -c < "$WATCHDOG_LOG")" -gt 1048576 ]; then
    : > "$WATCHDOG_LOG"
fi

# Check if configuration exists
[ ! -f "$CONFIG_FILE" ] && exit 0

# Ask the JSON-aware daemon helper; nested addon/node fields must not be
# mistaken for the root enabled flag. A missing binary is handled by start.sh.
if [ -x /tmp/unifi-bridge/unifi-bridge ]; then
    if /tmp/unifi-bridge/unifi-bridge -config "$CONFIG_FILE" -is-enabled; then
        :
    else
        enabled_result=$?
        [ "$enabled_result" -eq 3 ] && exit 0
        echo "Configuration could not be validated; watchdog did not restart the service" >> "$WATCHDOG_LOG"
        exit 1
    fi
fi

# Check if process is alive
PID=$(pidof unifi-bridge 2>/dev/null || true)
if [ -z "$PID" ]; then
    TIMESTAMP=$(date '+%Y-%m-%d %H:%M:%S')
    echo "[$TIMESTAMP] Watchdog: unifi-bridge is NOT running! Attempting restart..." >> "$WATCHDOG_LOG"
    /jffs/addons/unifi-bridge/start.sh >> "$WATCHDOG_LOG" 2>&1
    sleep 2
    NEW_PID=$(pidof unifi-bridge 2>/dev/null || true)
    if [ -n "$NEW_PID" ]; then
        echo "[$TIMESTAMP] Watchdog: unifi-bridge restarted successfully (PID: $NEW_PID)." >> "$WATCHDOG_LOG"
    else
        echo "[$TIMESTAMP] Watchdog: ERROR - failed to restart unifi-bridge!" >> "$WATCHDOG_LOG"
    fi
fi

# Refresh cluster status on Main AP
if [ -f "$INSTALL_DIR/sync_nodes.sh" ]; then
    "$INSTALL_DIR/sync_nodes.sh" status >/dev/null 2>&1 &
fi
