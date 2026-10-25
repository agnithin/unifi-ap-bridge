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
        BRANCH=$(echo "${2:-}" | sed -n 's/.*check_update_\([a-zA-Z0-9_-]*\)/\1/p')
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

        if [ -f "$SETTINGS_FILE" ]; then
            grep -v "^unifibridge_branch" "$SETTINGS_FILE" > "$SETTINGS_FILE.tmp" 2>/dev/null || true
            echo "unifibridge_branch $BRANCH" >> "$SETTINGS_FILE.tmp"
            mv -f "$SETTINGS_FILE.tmp" "$SETTINGS_FILE"
        fi

        if [ -f "$INSTALL_DIR/update.sh" ]; then
            "$INSTALL_DIR/update.sh" check "$BRANCH" >/tmp/unifi-bridge-update.log 2>&1 &
        fi
        exit 0
        ;;
    *do_update*)
        BRANCH=$(echo "${2:-}" | sed -n 's/.*do_update_\([a-zA-Z0-9_-]*\)/\1/p')
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

        if [ -f "$SETTINGS_FILE" ]; then
            grep -v "^unifibridge_branch" "$SETTINGS_FILE" > "$SETTINGS_FILE.tmp" 2>/dev/null || true
            echo "unifibridge_branch $BRANCH" >> "$SETTINGS_FILE.tmp"
            mv -f "$SETTINGS_FILE.tmp" "$SETTINGS_FILE"
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

# Parse values from amng_custom (NVRAM) if present
AMNG=$(nvram get amng_custom 2>/dev/null)
if [ -n "$AMNG" ]; then
    val=$(echo "$AMNG" | sed -n 's/.*"enabled"[[:space:]]*:[[:space:]]*\(true\|false\|1\|0\).*/\1/p')
    [ "$val" = "true" ] || [ "$val" = "1" ] && ENABLED="1"
    [ "$val" = "false" ] || [ "$val" = "0" ] && ENABLED="0"

    val=$(echo "$AMNG" | sed -n 's/.*"auto_gateway"[[:space:]]*:[[:space:]]*\(true\|false\|1\|0\).*/\1/p')
    [ "$val" = "true" ] || [ "$val" = "1" ] && AUTO_GW="1"
    [ "$val" = "false" ] || [ "$val" = "0" ] && AUTO_GW="0"

    val=$(echo "$AMNG" | sed -n 's/.*"controller_url"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')
    [ -n "$val" ] && URL="$val"

    val=$(echo "$AMNG" | sed -n 's/.*"model"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')
    [ -n "$val" ] && MODEL="$val"

    val=$(echo "$AMNG" | sed -n 's/.*"interval"[[:space:]]*:[[:space:]]*\([0-9]*\).*/\1/p')
    [ -n "$val" ] && INTERVAL="$val"

    val=$(echo "$AMNG" | sed -n 's/.*"branch"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')
    [ -n "$val" ] && BRANCH="$val"
fi

# Fallback: parse values from custom_settings.txt
if [ -f "$SETTINGS_FILE" ]; then
    val=$(grep -E "^unifibridge_enabled[[:space:]]+" "$SETTINGS_FILE" | tail -n 1 | awk '{print $2}')
    [ -n "$val" ] && [ -z "$AMNG" ] && ENABLED="$val"

    val=$(grep -E "^unifibridge_auto_gateway[[:space:]]+" "$SETTINGS_FILE" | tail -n 1 | awk '{print $2}')
    [ -n "$val" ] && [ -z "$AMNG" ] && AUTO_GW="$val"

    val=$(grep -E "^unifibridge_controller_url[[:space:]]+" "$SETTINGS_FILE" | tail -n 1 | awk '{print $2}')
    [ -n "$val" ] && [ -z "$AMNG" ] && URL="$val"

    val=$(grep -E "^unifibridge_model[[:space:]]+" "$SETTINGS_FILE" | tail -n 1 | awk '{print $2}')
    [ -n "$val" ] && [ -z "$AMNG" ] && MODEL="$val"

    val=$(grep -E "^unifibridge_interval[[:space:]]+" "$SETTINGS_FILE" | tail -n 1 | awk '{print $2}')
    [ -n "$val" ] && [ -z "$AMNG" ] && INTERVAL="$val"

    val=$(grep -E "^unifibridge_branch[[:space:]]+" "$SETTINGS_FILE" | tail -n 1 | awk '{print $2}')
    [ -n "$val" ] && [ -z "$AMNG" ] && BRANCH="$val"
fi

# Fallback: preserve existing branch or model from config.json if not set
if [ -f "$CONF_FILE" ]; then
    [ -z "$BRANCH" ] && BRANCH=$(grep '"branch"' "$CONF_FILE" 2>/dev/null | head -n 1 | awk -F'"' '{print $4}')
    [ -z "$MODEL" ] && MODEL=$(grep '"model"' "$CONF_FILE" 2>/dev/null | head -n 1 | awk -F'"' '{print $4}')
fi
[ -z "$BRANCH" ] && BRANCH="main"
[ -z "$MODEL" ] && MODEL="U6M"

# Sync custom_settings.txt with active branch
if [ -f "$SETTINGS_FILE" ]; then
    grep -v "^unifibridge_branch" "$SETTINGS_FILE" > "$SETTINGS_FILE.tmp" 2>/dev/null || true
    echo "unifibridge_branch $BRANCH" >> "$SETTINGS_FILE.tmp"
    mv -f "$SETTINGS_FILE.tmp" "$SETTINGS_FILE"
fi

# Detect Gateway IP if Auto
GW_IP=$(nvram get lan_gateway)
[ -z "$GW_IP" ] || [ "$GW_IP" = "0.0.0.0" ] && GW_IP="10.100.1.1"

if [ "$AUTO_GW" = "1" ] || [ "$AUTO_GW" = "true" ] || [ -z "$URL" ]; then
    URL="http://${GW_IP}:8080/inform"
fi

# Retain existing uplink_mac, uplink_remote_port, and wlan_mapping if present
UP_MAC=""
UP_PORT=0
WLAN_MAP='{"blueberry-prime":"6adb775a27327fe7fefdc22c","blueberry":"6adbb92027327fe7fefe9d60"}'

if [ -f "$CONF_FILE" ]; then
    UP_MAC=$(grep '"uplink_mac"' "$CONF_FILE" 2>/dev/null | head -n 1 | awk -F'"' '{print $4}')
    UP_PORT=$(grep '"uplink_remote_port"' "$CONF_FILE" 2>/dev/null | head -n 1 | awk -F':' '{print $2}' | tr -d ' ,\t\r\n')
fi

cat << EOF > "$CONF_FILE"
{
  "enabled": $( [ "$ENABLED" = "1" ] && echo "true" || echo "false" ),
  "auto_gateway": $( [ "$AUTO_GW" = "1" ] && echo "true" || echo "false" ),
  "controller_url": "$URL",
  "interval_sec": $INTERVAL,
  "model": "$MODEL",
  "branch": "$BRANCH",
  "state_path": "$INSTALL_DIR/state.json",
$([ -n "$UP_MAC" ] && echo "  \"uplink_mac\": \"$UP_MAC\",")
$([ -n "$UP_PORT" ] && [ "$UP_PORT" -gt 0 ] 2>/dev/null && echo "  \"uplink_remote_port\": $UP_PORT,")
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
