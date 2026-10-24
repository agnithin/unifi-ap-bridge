#!/bin/sh
# ==============================================================================
# UniFi AP Bridge - Apply Configuration Handler for Asuswrt-Merlin
# ==============================================================================

INSTALL_DIR="/jffs/addons/unifi-bridge"
CONF_DIR="$INSTALL_DIR/config"
CONF_FILE="$CONF_DIR/config.json"
SETTINGS_FILE="/jffs/addons/custom_settings.txt"

mkdir -p "$CONF_DIR"

# Actions triggered via service-event
case "${2:-}" in
    *check_update*)
        BRANCH="main"
        if [ -f "$SETTINGS_FILE" ]; then
            val=$(grep -E "^unifibridge_branch[[:space:]]+" "$SETTINGS_FILE" | tail -n 1 | awk '{print $2}')
            [ -n "$val" ] && BRANCH="$val"
        fi
        if [ -f "$INSTALL_DIR/update.sh" ]; then
            "$INSTALL_DIR/update.sh" check "$BRANCH" >/tmp/unifi-bridge-update.log 2>&1 &
        fi
        exit 0
        ;;
    *do_update*)
        BRANCH="main"
        if [ -f "$SETTINGS_FILE" ]; then
            val=$(grep -E "^unifibridge_branch[[:space:]]+" "$SETTINGS_FILE" | tail -n 1 | awk '{print $2}')
            [ -n "$val" ] && BRANCH="$val"
        fi
        if [ -f "$INSTALL_DIR/update.sh" ]; then
            "$INSTALL_DIR/update.sh" apply "$BRANCH" >/tmp/unifi-bridge-update.log 2>&1 &
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

# Defaults
ENABLED="1"
AUTO_GW="1"
URL=""
MODEL="U6M"
INTERVAL="10"
BRANCH="main"

# Parse values from custom_settings.txt
if [ -f "$SETTINGS_FILE" ]; then
    val=$(grep -E "^unifibridge_enabled[[:space:]]+" "$SETTINGS_FILE" | tail -n 1 | awk '{print $2}')
    [ -n "$val" ] && ENABLED="$val"

    val=$(grep -E "^unifibridge_auto_gateway[[:space:]]+" "$SETTINGS_FILE" | tail -n 1 | awk '{print $2}')
    [ -n "$val" ] && AUTO_GW="$val"

    val=$(grep -E "^unifibridge_controller_url[[:space:]]+" "$SETTINGS_FILE" | tail -n 1 | awk '{print $2}')
    [ -n "$val" ] && URL="$val"

    val=$(grep -E "^unifibridge_model[[:space:]]+" "$SETTINGS_FILE" | tail -n 1 | awk '{print $2}')
    [ -n "$val" ] && MODEL="$val"

    val=$(grep -E "^unifibridge_interval[[:space:]]+" "$SETTINGS_FILE" | tail -n 1 | awk '{print $2}')
    [ -n "$val" ] && INTERVAL="$val"

    val=$(grep -E "^unifibridge_branch[[:space:]]+" "$SETTINGS_FILE" | tail -n 1 | awk '{print $2}')
    [ -n "$val" ] && BRANCH="$val"
fi

# Detect Gateway IP if Auto
GW_IP=$(nvram get lan_gateway)
[ -z "$GW_IP" ] || [ "$GW_IP" = "0.0.0.0" ] && GW_IP="10.100.1.1"

if [ "$AUTO_GW" = "1" ] || [ "$AUTO_GW" = "true" ] || [ -z "$URL" ]; then
    URL="http://${GW_IP}:8080/inform"
fi

# Retain existing wlan_mapping if present
WLAN_MAP='{"blueberry-prime":"6adb775a27327fe7fefdc22c","blueberry":"6adbb92027327fe7fefe9d60"}'

cat << EOF > "$CONF_FILE"
{
  "enabled": $( [ "$ENABLED" = "1" ] && echo "true" || echo "false" ),
  "auto_gateway": $( [ "$AUTO_GW" = "1" ] && echo "true" || echo "false" ),
  "controller_url": "$URL",
  "interval_sec": $INTERVAL,
  "model": "$MODEL",
  "branch": "$BRANCH",
  "state_path": "$INSTALL_DIR/state.json",
  "wlan_mapping": $WLAN_MAP
}
EOF

# Restart daemon if enabled, or kill if disabled
if [ "$ENABLED" = "1" ]; then
    "$INSTALL_DIR/start.sh"
else
    killall unifi-bridge 2>/dev/null || true
    mkdir -p /tmp/var/wwwext/unifi-bridge
    cat << EOF > /tmp/var/wwwext/unifi-bridge/status.js
window.unifiBridgeStatus = {
  "running": false,
  "last_status": "Disabled by user in WebUI",
  "total_stations": 0,
  "stations_2g": 0,
  "stations_5g": 0,
  "recent_logs": ["[$(date +%T)] Bridge daemon stopped by user"]
};
EOF
fi

# Propagate new configuration to Satellite Nodes
if [ -f "$INSTALL_DIR/sync_nodes.sh" ]; then
    "$INSTALL_DIR/sync_nodes.sh" sync >/tmp/unifi-bridge-sync.log 2>&1 &
fi
