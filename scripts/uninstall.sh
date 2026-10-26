#!/bin/sh
set -eu

ADDON_NAME=unifi-bridge
INSTALL_DIR="/jffs/addons/$ADDON_NAME"
SCRIPTS_DIR=/jffs/scripts
WWW_DIR=/tmp/var/wwwext

slot=
if [ -f "$INSTALL_DIR/.user_slot" ]; then slot=$(tr -d ' \t\r\n' < "$INSTALL_DIR/.user_slot"); fi
case "$slot" in user[1-9]|user1[0-9]|user20|'') ;;
    *) echo "Uninstaller: refusing invalid saved WebUI slot" >&2; exit 1 ;;
esac

remove_owned_hook_lines() {
    file=$1; tmp="$file.uninstall.$$"
    [ -f "$file" ] || return 0
    start_line="$INSTALL_DIR/start.sh </dev/null >/dev/null 2>&1 &"
    stop_line='killall unifi-bridge 2>/dev/null || true'
    event_line='if echo "$2" | /bin/grep -q "unifi_bridge"; then { /jffs/addons/unifi-bridge/apply_config.sh "$@" & }; fi'
    awk -v start="$start_line" -v stop="$stop_line" -v event="$event_line" '
      $0 == "# BEGIN unifi-ap-bridge" { skip=1; next }
      $0 == "# END unifi-ap-bridge" { skip=0; next }
      skip { next }
      $0 == "# UniFi AP Bridge Addon" || $0 == start || $0 == stop || $0 == event { next }
      { print }
    ' "$file" > "$tmp"
    chmod 755 "$tmp"
    mv -f "$tmp" "$file"
}

if type cru >/dev/null 2>&1; then cru d unifi_bridge_watchdog 2>/dev/null || true; fi
killall unifi-bridge 2>/dev/null || true
for hook in services-start services-stop service-event; do
    remove_owned_hook_lines "$SCRIPTS_DIR/$hook"
done

# Remove the CLI link only when it still points into this addon's directory.
if [ -L "$SCRIPTS_DIR/unifi-bridge" ] &&
   [ "$(readlink "$SCRIPTS_DIR/unifi-bridge" 2>/dev/null || true)" = "$INSTALL_DIR/unifi-bridge.sh" ]; then
    rm -f "$SCRIPTS_DIR/unifi-bridge"
fi
if [ -L /opt/bin/unifi-bridge ] &&
   [ "$(readlink /opt/bin/unifi-bridge 2>/dev/null || true)" = "$SCRIPTS_DIR/unifi-bridge" ]; then
    rm -f /opt/bin/unifi-bridge
fi

remove_owned_slot() {
    slot_name=$1
    page="$WWW_DIR/$slot_name.asp"
    title="$WWW_DIR/$slot_name.title"
    if [ -f "$page" ] && [ ! -L "$page" ] && grep -q 'unifi-ap-bridge-webui' "$page" 2>/dev/null &&
       [ -f "$title" ] && [ ! -L "$title" ] && grep -Fxq 'UniFi AP Bridge' "$title" 2>/dev/null; then
        rm -f "$page" "$title"
    fi
}
if [ -n "$slot" ]; then
    remove_owned_slot "$slot"
else
    # Older installs may lack the saved slot file. Recover only pages bearing
    # this addon's embedded marker, never by assuming user3 or a fixed index.
    i=1
    while [ "$i" -le 20 ]; do remove_owned_slot "user$i"; i=$((i + 1)); done
fi
rm -rf "$WWW_DIR/unifi-bridge"

# Remove only our own menu object, retaining every other menuTree entry.
if [ -f /tmp/menuTree.js ] && grep -q 'tabName: "UniFi AP Bridge"' /tmp/menuTree.js; then
    awk '{ gsub(/\{url: "user[0-9][0-9]*\.asp", tabName: "UniFi AP Bridge"\},?/, "", $0); print }' /tmp/menuTree.js > /tmp/menuTree.js.uninstall.$$
    mv -f /tmp/menuTree.js.uninstall.$$ /tmp/menuTree.js
    umount /www/require/modules/menuTree.js 2>/dev/null || true
    mount -o bind /tmp/menuTree.js /www/require/modules/menuTree.js 2>/dev/null || true
fi

rm -rf "$INSTALL_DIR" "/tmp/$ADDON_NAME"
rm -f /tmp/unifi-bridge.log /tmp/unifi-bridge-watchdog.log
echo "UniFi AP Bridge uninstalled."
