#!/bin/sh
set -eu

# ==============================================================================
# unifi-ap-bridge Uninstaller for Asuswrt-Merlin
# ==============================================================================

ADDON_NAME="unifi-bridge"
INSTALL_DIR="/jffs/addons/$ADDON_NAME"
SCRIPTS_DIR="/jffs/scripts"

echo "====================================================="
echo "   Uninstalling UniFi AP Bridge                      "
echo "====================================================="

# 1. Disarm watchdog cron
if type cru >/dev/null 2>&1; then
    cru d unifi_bridge_watchdog 2>/dev/null || true
fi

# 2. Stop running daemon
killall unifi-bridge 2>/dev/null || true

# 3. Clean up services-start
if [ -f "$SCRIPTS_DIR/services-start" ]; then
    sed -i '/# UniFi AP Bridge Addon/d' "$SCRIPTS_DIR/services-start"
    sed -i '/unifi-bridge/d' "$SCRIPTS_DIR/services-start"
fi

# 4. Clean up services-stop
if [ -f "$SCRIPTS_DIR/services-stop" ]; then
    sed -i '/# UniFi AP Bridge Addon/d' "$SCRIPTS_DIR/services-stop"
    sed -i '/unifi-bridge/d' "$SCRIPTS_DIR/services-stop"
fi

# 5. Clean up service-event
if [ -f "$SCRIPTS_DIR/service-event" ]; then
    sed -i '/unifi_bridge/d' "$SCRIPTS_DIR/service-event"
fi

# 6. Remove CLI management companion
rm -f "$SCRIPTS_DIR/unifi-bridge"
rm -f "/opt/bin/unifi-bridge"

# 7. Remove WebUI files and menuTree entries
USER_SLOT="user3"
if [ -f "$INSTALL_DIR/.user_slot" ]; then
    USER_SLOT=$(cat "$INSTALL_DIR/.user_slot" 2>/dev/null | tr -d ' \t\r\n')
fi

rm -f "/tmp/var/wwwext/${USER_SLOT}.asp"
rm -f "/tmp/var/wwwext/${USER_SLOT}.title"
rm -rf "/tmp/var/wwwext/unifi-bridge"

if [ -f /tmp/menuTree.js ]; then
    sed -i "/\${USER_SLOT}\.asp/d" /tmp/menuTree.js
    sed -i "/UniFi AP Bridge/d" /tmp/menuTree.js
    umount /www/require/modules/menuTree.js 2>/dev/null || true
    mount -o bind /tmp/menuTree.js /www/require/modules/menuTree.js 2>/dev/null || true
fi

# 8. Remove installation files
rm -rf "$INSTALL_DIR"
rm -rf "/tmp/$ADDON_NAME"
rm -f "/tmp/unifi-bridge.log"
rm -f "/tmp/unifi-bridge-watchdog.log"

echo "====================================================="
echo "   UniFi AP Bridge uninstalled cleanly.              "
echo "====================================================="
