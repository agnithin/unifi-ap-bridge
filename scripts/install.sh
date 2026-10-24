#!/bin/sh
set -eu

# ==============================================================================
# unifi-ap-bridge Installer for Asuswrt-Merlin
# ==============================================================================

ADDON_NAME="unifi-bridge"
INSTALL_DIR="/jffs/addons/$ADDON_NAME"
BIN_DIR="$INSTALL_DIR/bin"
CONFIG_DIR="$INSTALL_DIR/config"
WEBUI_DIR="$INSTALL_DIR/webui"
SCRIPTS_DIR="/jffs/scripts"
TMP_DIR="/tmp/$ADDON_NAME"

echo "====================================================="
echo "   Installing UniFi AP Bridge for Asuswrt-Merlin     "
echo "====================================================="

mkdir -p "$BIN_DIR" "$CONFIG_DIR" "$WEBUI_DIR" "$TMP_DIR"

# 1. Stop existing service if running
killall unifi-bridge 2>/dev/null || true

# 2. Determine architecture and install binary
ARCH=$(uname -m)
echo "[*] Detected hardware architecture: $ARCH"

case "$ARCH" in
    armv7l)
        BIN_NAME="unifi-bridge-armv7"
        ;;
    aarch64)
        BIN_NAME="unifi-bridge-arm64"
        ;;
    *)
        BIN_NAME="unifi-bridge-armv7"
        echo "[!] Warning: unrecognized architecture $ARCH, defaulting to armv7"
        ;;
esac

if [ -f "./bin/$BIN_NAME" ]; then
    echo "[*] Installing local $BIN_NAME binary..."
    cp "./bin/$BIN_NAME" "$BIN_DIR/unifi-bridge"
    chmod 755 "$BIN_DIR/unifi-bridge"
elif [ -f "./bin/unifi-bridge" ]; then
    cp "./bin/unifi-bridge" "$BIN_DIR/unifi-bridge"
    chmod 755 "$BIN_DIR/unifi-bridge"
elif [ ! -f "$BIN_DIR/unifi-bridge" ]; then
    echo "[*] Downloading latest unifi-bridge binary for $ARCH..."
    curl -fsSL "https://github.com/agnithin/unifi-ap-bridge/releases/latest/download/$BIN_NAME" -o "$BIN_DIR/unifi-bridge"
    chmod 755 "$BIN_DIR/unifi-bridge"
fi

# 3. Dynamic WebUI Slot Allocation
find_user_slot() {
    SLOT_FILE="$INSTALL_DIR/.user_slot"
    if [ -f "$SLOT_FILE" ]; then
        saved_slot=$(cat "$SLOT_FILE" 2>/dev/null | tr -d ' \t\r\n')
        if [ -n "$saved_slot" ]; then
            echo "$saved_slot"
            return
        fi
    fi

    for i in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20; do
        target="/tmp/var/wwwext/user${i}.asp"
        title="/tmp/var/wwwext/user${i}.title"
        if [ -f "$target" ] && grep -q "unifi-ap-bridge-webui" "$target" 2>/dev/null; then
            echo "user${i}"
            echo "user${i}" > "$SLOT_FILE" 2>/dev/null || true
            return
        fi
        if [ -f "$title" ] && grep -qiE "unifi.*bridge" "$title" 2>/dev/null; then
            echo "user${i}"
            echo "user${i}" > "$SLOT_FILE" 2>/dev/null || true
            return
        fi
    done

    for i in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20; do
        target="/tmp/var/wwwext/user${i}.asp"
        title="/tmp/var/wwwext/user${i}.title"
        if [ ! -f "$target" ] && [ ! -f "$title" ]; then
            echo "user${i}"
            echo "user${i}" > "$SLOT_FILE" 2>/dev/null || true
            return
        fi
    done

    echo "user3"
}

USER_SLOT=$(find_user_slot)
echo "[*] Allocated WebUI slot: $USER_SLOT.asp"

# Install WebUI ASP page & Apply handler
if [ -f "./webui/unifi-bridge.asp" ]; then
    echo "[*] Installing WebUI ASP page..."
    cp "./webui/unifi-bridge.asp" "$WEBUI_DIR/unifi-bridge.asp"
fi

if [ -f "./scripts/apply_config.sh" ]; then
    cp "./scripts/apply_config.sh" "$INSTALL_DIR/apply_config.sh"
    chmod 755 "$INSTALL_DIR/apply_config.sh"
fi

if [ -f "./scripts/watchdog.sh" ]; then
    cp "./scripts/watchdog.sh" "$INSTALL_DIR/watchdog.sh"
    chmod 755 "$INSTALL_DIR/watchdog.sh"
fi

if [ -f "./scripts/sync_nodes.sh" ]; then
    cp "./scripts/sync_nodes.sh" "$INSTALL_DIR/sync_nodes.sh"
    chmod 755 "$INSTALL_DIR/sync_nodes.sh"
fi

if [ -f "./scripts/update.sh" ]; then
    cp "./scripts/update.sh" "$INSTALL_DIR/update.sh"
    chmod 755 "$INSTALL_DIR/update.sh"
fi

if [ -f "./version.txt" ]; then
    cp "./version.txt" "$INSTALL_DIR/version.txt"
    chmod 644 "$INSTALL_DIR/version.txt"
fi

# Install CLI management companion to /jffs/scripts/unifi-bridge
if [ -f "./scripts/unifi-bridge.sh" ]; then
    echo "[*] Installing CLI manager companion to /jffs/scripts/unifi-bridge..."
    cp "./scripts/unifi-bridge.sh" "$SCRIPTS_DIR/unifi-bridge"
    chmod 755 "$SCRIPTS_DIR/unifi-bridge"
    # Entware symlink if available
    [ -d "/opt/bin" ] && ln -sf "$SCRIPTS_DIR/unifi-bridge" /opt/bin/unifi-bridge 2>/dev/null || true
fi

# 4. Generate persistent start.sh
cat << 'STARTEOF' > "$INSTALL_DIR/start.sh"
#!/bin/sh
INSTALL_DIR="/jffs/addons/unifi-bridge"
mkdir -p /tmp/unifi-bridge
if [ ! -f /tmp/unifi-bridge/unifi-bridge ] || [ "$INSTALL_DIR/unifi-bridge.tar.gz" -nt /tmp/unifi-bridge/unifi-bridge ]; then
    if [ -f "$INSTALL_DIR/unifi-bridge.tar.gz" ]; then
        tar -xzf "$INSTALL_DIR/unifi-bridge.tar.gz" -C /tmp/unifi-bridge
        [ -f /tmp/unifi-bridge/unifi-bridge-armv7 ] && mv -f /tmp/unifi-bridge/unifi-bridge-armv7 /tmp/unifi-bridge/unifi-bridge
        [ -f /tmp/unifi-bridge/unifi-bridge-arm64 ] && mv -f /tmp/unifi-bridge/unifi-bridge-arm64 /tmp/unifi-bridge/unifi-bridge
    elif [ -f "$INSTALL_DIR/bin/unifi-bridge" ]; then
        cp -fp "$INSTALL_DIR/bin/unifi-bridge" /tmp/unifi-bridge/unifi-bridge
    fi
    chmod 755 /tmp/unifi-bridge/unifi-bridge
fi

# WebUI mount
SLOT_FILE="$INSTALL_DIR/.user_slot"
USER_SLOT="user3"
[ -f "$SLOT_FILE" ] && USER_SLOT=$(cat "$SLOT_FILE" 2>/dev/null | tr -d ' \t\r\n')
[ -z "$USER_SLOT" ] && USER_SLOT="user3"

mkdir -p /tmp/var/wwwext/unifi-bridge
if [ -f "$INSTALL_DIR/webui/unifi-bridge.asp" ]; then
    cp -fp "$INSTALL_DIR/webui/unifi-bridge.asp" "/tmp/var/wwwext/${USER_SLOT}.asp"
    echo "UniFi AP Bridge" > "/tmp/var/wwwext/${USER_SLOT}.title"
fi

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

killall unifi-bridge 2>/dev/null || true
nohup /tmp/unifi-bridge/unifi-bridge -config "$INSTALL_DIR/config/config.json" >/tmp/unifi-bridge.log 2>&1 &
STARTEOF
chmod 755 "$INSTALL_DIR/start.sh"

# 5. Create default config if missing
CONFIG_FILE="$CONFIG_DIR/config.json"
GW_IP=$(nvram get lan_gateway)
[ -z "$GW_IP" ] || [ "$GW_IP" = "0.0.0.0" ] && GW_IP="10.100.1.1"

if [ ! -f "$CONFIG_FILE" ]; then
    echo "[*] Generating initial config with auto-detected gateway ($GW_IP)..."
    cat << EOF > "$CONFIG_FILE"
{
  "enabled": true,
  "auto_gateway": true,
  "controller_url": "http://${GW_IP}:8080/inform",
  "interval_sec": 10,
  "model": "U6M",
  "state_path": "$INSTALL_DIR/state.json",
  "wlan_mapping": {
    "blueberry-prime": "6adb775a27327fe7fefdc22c",
    "blueberry": "6adbb92027327fe7fefe9d60"
  }
}
EOF
    chmod 600 "$CONFIG_FILE"
fi

# 6. Hook into /jffs/scripts/services-start
SERVICES_START="$SCRIPTS_DIR/services-start"
START_HOOK="$INSTALL_DIR/start.sh </dev/null >/dev/null 2>&1 &"

if [ ! -f "$SERVICES_START" ]; then
    printf '#!/bin/sh\n' > "$SERVICES_START"
    chmod 755 "$SERVICES_START"
fi

if ! grep -q "unifi-bridge" "$SERVICES_START"; then
    echo "[*] Adding startup hook to $SERVICES_START..."
    printf '\n# UniFi AP Bridge Addon\n%s\n' "$START_HOOK" >> "$SERVICES_START"
fi

# 7. Hook into /jffs/scripts/services-stop
SERVICES_STOP="$SCRIPTS_DIR/services-stop"
STOP_HOOK="killall unifi-bridge 2>/dev/null || true"

if [ ! -f "$SERVICES_STOP" ]; then
    printf '#!/bin/sh\n' > "$SERVICES_STOP"
    chmod 755 "$SERVICES_STOP"
fi

if ! grep -q "unifi-bridge" "$SERVICES_STOP"; then
    echo "[*] Adding shutdown hook to $SERVICES_STOP..."
    printf '\n# UniFi AP Bridge Addon\n%s\n' "$STOP_HOOK" >> "$SERVICES_STOP"
fi

# 8. Hook into /jffs/scripts/service-event for WebUI applies
SERVICE_EVENT="$SCRIPTS_DIR/service-event"
EVENT_HOOK='if echo "$2" | /bin/grep -q "unifi_bridge"; then { /jffs/addons/unifi-bridge/apply_config.sh "$@" & }; fi # UniFi Bridge'

if [ ! -f "$SERVICE_EVENT" ]; then
    printf '#!/bin/sh\n' > "$SERVICE_EVENT"
    chmod 755 "$SERVICE_EVENT"
fi

if ! grep -q "unifi_bridge" "$SERVICE_EVENT"; then
    echo "[*] Adding service-event hook to $SERVICE_EVENT..."
    printf '%s\n' "$EVENT_HOOK" >> "$SERVICE_EVENT"
fi

# 9. Start the daemon now
echo "[*] Starting unifi-bridge daemon..."
"$INSTALL_DIR/start.sh"

echo "====================================================="
echo "   UniFi AP Bridge installed successfully!           "
echo "   WebUI: http://$(nvram get lan_ipaddr)/${USER_SLOT}.asp"
echo "   CLI:   unifi-bridge (or /jffs/scripts/unifi-bridge) "
echo "   Logs:  /tmp/unifi-bridge.log                      "
echo "====================================================="
