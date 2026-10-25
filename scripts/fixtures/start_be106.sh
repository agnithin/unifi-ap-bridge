#!/bin/sh
INSTALL_DIR="/jffs/addons/unifi-bridge"
CONFIG_FILE="$INSTALL_DIR/config/config.json"
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

# Guard boot, cluster and manual entry points after preparing the binary. The
# daemon publishes complete disabled metadata without accessing Wi-Fi/adoption.
if /tmp/unifi-bridge/unifi-bridge -config "$CONFIG_FILE" -is-enabled; then
    :
else
    enabled_result=$?
    [ "$enabled_result" -eq 3 ] || exit "$enabled_result"
    killall unifi-bridge 2>/dev/null || true
    /tmp/unifi-bridge/unifi-bridge -config "$CONFIG_FILE" -once
    exit $?
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
