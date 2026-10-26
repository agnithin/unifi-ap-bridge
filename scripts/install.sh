#!/bin/sh
set -eu

# Install only from the flat, complete, same-generation release bundle. The
# release workflow places this script beside every asset and bundle.json.
ADDON_NAME="unifi-bridge"
INSTALL_DIR="/jffs/addons/$ADDON_NAME"
SCRIPTS_DIR="/jffs/scripts"
TMP_DIR="/tmp/$ADDON_NAME"
SOURCE_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)

fail() { echo "Installer: $*" >&2; exit 1; }
read_meta() {
    # bundle.json is generated with one scalar per line. Reject duplicate or
    # malformed keys instead of sourcing the JSON as shell code.
    key=$1
    awk -F '"' -v wanted="$key" '$2 == wanted && $3 ~ /^[[:space:]]*:[[:space:]]*$/ { count++; value=$4 } END { if (count == 1) print value; else exit 1 }' "$SOURCE_DIR/bundle.json"
}
sha256_file() {
    [ -f "$1" ] || return 1
    openssl dgst -sha256 "$1" 2>/dev/null | awk '{print $NF}'
}
valid_hex() { [ "${#1}" -eq 64 ] && case "$1" in *[!0-9a-f]*) return 1;; esac; }

case "$(uname -m)" in
    armv7l) ARCH=armv7; BIN_NAME=unifi-bridge-armv7 ;;
    aarch64) ARCH=arm64; BIN_NAME=unifi-bridge-arm64 ;;
    *) fail "unsupported architecture $(uname -m); no files changed" ;;
esac

REQUIRED="apply_config.sh install.sh start.sh sync_nodes.sh unifi-bridge.sh update.sh watchdog.sh version.txt unifi-bridge.asp bundle.json $BIN_NAME"
for file in $REQUIRED; do
    [ -f "$SOURCE_DIR/$file" ] && [ ! -L "$SOURCE_DIR/$file" ] && [ -r "$SOURCE_DIR/$file" ] || fail "missing or non-regular release member $file; no files changed"
done

META_KEYS="version commit architecture"
for file in $REQUIRED; do
    [ "$file" = bundle.json ] || META_KEYS="$META_KEYS sha256_$file"
done
validate_metadata() {
    awk -F '"' -v expected="$META_KEYS" '
      BEGIN { n=split(expected, keys, " "); for (i=1; i<=n; i++) allowed[keys[i]]=1 }
      NR == 1 { if ($0 != "{") bad=1; next }
      NR == n+2 { if ($0 != "}") bad=1; closed=1; next }
      NR > n+2 { bad=1; next }
      {
        if (NR < 2 || NR > n+1 || $1 != "  " || NF != 5 || $3 != ": ") bad=1
        key=$2
        if (!(key in allowed) || seen[key]++) bad=1
        if (NR < n+1 && $5 != ",") bad=1
        if (NR == n+1 && $5 != "") bad=1
      }
      END { if (bad || !closed) exit 1; for (i=1; i<=n; i++) if (!seen[keys[i]]) exit 1 }
    ' "$SOURCE_DIR/bundle.json"
}
validate_metadata || fail "bundle.json is malformed or contains unknown/duplicate metadata; no files changed"

META_VERSION=$(read_meta version) || fail "invalid bundle version metadata; no files changed"
META_COMMIT=$(read_meta commit) || fail "invalid bundle commit metadata; no files changed"
META_ARCH=$(read_meta architecture) || fail "invalid bundle architecture metadata; no files changed"
printf '%s\n' "$META_VERSION" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$' || fail "invalid bundle version; no files changed"
case "$META_COMMIT" in *[!0-9a-f]*|'') fail "invalid bundle commit; no files changed";; esac
[ "${#META_COMMIT}" -eq 40 ] || fail "invalid bundle commit; no files changed"
[ "$META_ARCH" = "$ARCH" ] || fail "bundle architecture $META_ARCH does not match device $ARCH; no files changed"
LOCAL_VERSION=$(tr -d ' \t\r\n' < "$SOURCE_DIR/version.txt")
[ "$LOCAL_VERSION" = "$META_VERSION" ] || fail "version.txt and bundle.json come from different generations; no files changed"

# The manifest is embedded in bundle.json so extraction of the published flat
# bundle remains independently checkable and does not extend updater allowlists.
for file in $REQUIRED; do
    [ "$file" = bundle.json ] && continue
    expected=$(read_meta "sha256_$file") || fail "missing checksum for $file; no files changed"
    valid_hex "$expected" || fail "invalid checksum for $file; no files changed"
    actual=$(sha256_file "$SOURCE_DIR/$file") || fail "cannot checksum $file; no files changed"
    [ "$expected" = "$actual" ] || fail "checksum mismatch for $file; no files changed"
done
IDENTITY=$("$SOURCE_DIR/$BIN_NAME" -version 2>/dev/null) || fail "release daemon cannot report identity; no files changed"
printf '%s\n' "$IDENTITY" | grep -Fq "unifi-bridge $META_VERSION (commit: $META_COMMIT," || fail "binary identity does not match bundle metadata; no files changed"

# Validate all shell assets before touching persistent paths or stopping service.
for file in apply_config.sh start.sh sync_nodes.sh unifi-bridge.sh update.sh watchdog.sh install.sh; do
    sh -n "$SOURCE_DIR/$file" || fail "shell syntax error in $file; no files changed"
done

# Select and validate a menu slot before mutation. Never fall back to a slot
# owned by another addon when the pool is exhausted.
SLOT_FILE="$INSTALL_DIR/.user_slot"
USER_SLOT=
slot_owned() {
    candidate=$1
    page="/tmp/var/wwwext/$candidate.asp"
    title="/tmp/var/wwwext/$candidate.title"
    [ -f "$page" ] && [ ! -L "$page" ] && grep -q 'unifi-ap-bridge-webui' "$page" 2>/dev/null &&
        [ -f "$title" ] && [ ! -L "$title" ] && grep -Fxq 'UniFi AP Bridge' "$title" 2>/dev/null
}
if [ -f "$SLOT_FILE" ]; then
    USER_SLOT=$(tr -d ' \t\r\n' < "$SLOT_FILE")
    case "$USER_SLOT" in user[1-9]|user1[0-9]|user20) ;; *) fail "saved WebUI slot is invalid; no files changed";; esac
    saved_page="/tmp/var/wwwext/$USER_SLOT.asp"
    saved_title="/tmp/var/wwwext/$USER_SLOT.title"
    if [ -e "$saved_page" ] || [ -L "$saved_page" ] || [ -e "$saved_title" ] || [ -L "$saved_title" ]; then
        slot_owned "$USER_SLOT" || fail "saved WebUI slot is now owned by another addon; no files changed"
    fi
else
    i=1
    while [ "$i" -le 20 ]; do
        candidate="user$i"
        if slot_owned "$candidate"; then USER_SLOT="$candidate"; break; fi
        i=$((i + 1))
    done
    if [ -z "$USER_SLOT" ]; then
        i=1
        while [ "$i" -le 20 ]; do
            target="/tmp/var/wwwext/user${i}.asp"
            title="/tmp/var/wwwext/user${i}.title"
            if [ ! -e "$target" ] && [ ! -L "$target" ] && [ ! -e "$title" ] && [ ! -L "$title" ]; then USER_SLOT="user$i"; break; fi
            i=$((i + 1))
        done
    fi
    [ -n "$USER_SLOT" ] || fail "no free WebUI user slot; no files changed"
fi

# Validate the gateway as four decimal octets before JSON serialization.
GW_IP=$(nvram get lan_gateway 2>/dev/null || true)
valid_ipv4() {
    case "$1" in *[!0-9.]*|.*|*.) return 1;; esac
    old_ifs=$IFS; IFS=.; set -- $1; IFS=$old_ifs
    [ "$#" -eq 4 ] || return 1
    for octet do
        case "$octet" in ''|*[!0-9]*) return 1;; esac
        [ "${#octet}" -le 3 ] || return 1
        [ "${#octet}" -eq 1 ] || [ "${octet#0}" = "$octet" ] || return 1
        [ "$octet" -le 255 ] || return 1
    done
}
if ! valid_ipv4 "$GW_IP" || [ "$GW_IP" = "0.0.0.0" ]; then GW_IP=10.100.1.1; fi

# Build the exact archive consumed by start.sh from the already verified flat
# candidate. This prevents a previous installed tarball from winning over the
# newly verified standalone binary during a reinstall.
STAGE="$TMP_DIR/install-stage-$$"
mkdir -p "$STAGE" || fail "cannot create RAM staging directory; no files changed"
trap 'rm -rf "$STAGE"' EXIT HUP INT TERM
ARCHIVE_MEMBERS="$BIN_NAME install.sh start.sh apply_config.sh sync_nodes.sh unifi-bridge.sh update.sh watchdog.sh unifi-bridge.asp version.txt bundle.json"
COPYFILE_DISABLE=1 tar -czf "$STAGE/unifi-bridge.tar.gz" -C "$SOURCE_DIR" $ARCHIVE_MEMBERS || fail "cannot stage verified generation; no files changed"
printf '%s\n' $ARCHIVE_MEMBERS | LC_ALL=C sort > "$STAGE/members.expected"
tar -tzf "$STAGE/unifi-bridge.tar.gz" | LC_ALL=C sort > "$STAGE/members.actual" || fail "cannot inspect staged generation; no files changed"
cmp -s "$STAGE/members.expected" "$STAGE/members.actual" || fail "staged generation member set is incomplete; no files changed"
ARCHIVE_SHA=$(sha256_file "$STAGE/unifi-bridge.tar.gz") || fail "cannot checksum staged generation; no files changed"
printf '%s  unifi-bridge.tar.gz\n' "$ARCHIVE_SHA" > "$STAGE/unifi-bridge.tar.gz.sha256"

# Only after every artifact, schema, architecture, and slot check passes may
# persistent state be prepared or a running daemon be stopped.
mkdir -p "$INSTALL_DIR" "$INSTALL_DIR/.ssh" "$INSTALL_DIR/config" \
    "$INSTALL_DIR/webui" "$TMP_DIR" "$SCRIPTS_DIR" || fail "cannot prepare install directories"
chmod 755 "$INSTALL_DIR"
chmod 700 "$INSTALL_DIR/.ssh"

CONFIG_FILE="$INSTALL_DIR/config/config.json"
CONFIG_TMP="$CONFIG_FILE.new.$$"
if [ ! -f "$CONFIG_FILE" ]; then
    printf '{\n  "enabled": true,\n  "controller_url": "http://%s:8080/inform",\n  "interval_sec": 10,\n  "model": "U6M",\n  "state_path": "%s/state.json",\n  "wlan_mapping": {}\n}\n' "$GW_IP" "$INSTALL_DIR" > "$CONFIG_TMP" || fail "cannot write initial configuration"
    chmod 600 "$CONFIG_TMP"
    mv -f "$CONFIG_TMP" "$CONFIG_FILE"
fi

killall unifi-bridge 2>/dev/null || true
install_file() { cp -p "$1" "$2" || fail "cannot install $3"; }
for file in apply_config.sh start.sh sync_nodes.sh unifi-bridge.sh update.sh watchdog.sh; do
    install_file "$SOURCE_DIR/$file" "$INSTALL_DIR/$file" "$file"
    chmod 755 "$INSTALL_DIR/$file"
done
install_file "$SOURCE_DIR/version.txt" "$INSTALL_DIR/version.txt" version.txt
chmod 644 "$INSTALL_DIR/version.txt"
install_file "$SOURCE_DIR/unifi-bridge.asp" "$INSTALL_DIR/webui/unifi-bridge.asp" WebUI
chmod 644 "$INSTALL_DIR/webui/unifi-bridge.asp"
install_file "$STAGE/unifi-bridge.tar.gz" "$INSTALL_DIR/unifi-bridge.tar.gz" archive
install_file "$STAGE/unifi-bridge.tar.gz.sha256" "$INSTALL_DIR/unifi-bridge.tar.gz.sha256" archive-checksum
chmod 644 "$INSTALL_DIR/unifi-bridge.tar.gz" "$INSTALL_DIR/unifi-bridge.tar.gz.sha256"
printf '%s\n' "$USER_SLOT" > "$SLOT_FILE.new.$$"
chmod 600 "$SLOT_FILE.new.$$"
mv -f "$SLOT_FILE.new.$$" "$SLOT_FILE"

ln -sf "$INSTALL_DIR/unifi-bridge.sh" "$SCRIPTS_DIR/unifi-bridge"
[ ! -d /opt/bin ] || ln -sf "$SCRIPTS_DIR/unifi-bridge" /opt/bin/unifi-bridge 2>/dev/null || true

upsert_hook() {
    file=$1; body=$2; tmp="$file.new.$$"
    [ -f "$file" ] || printf '#!/bin/sh\n' > "$file"
    awk -v own="$body" '
      $0 == "# BEGIN unifi-ap-bridge" { skip=1; next }
      $0 == "# END unifi-ap-bridge" { skip=0; next }
      skip { next }
      $0 == own { next }
      { print }
    ' "$file" > "$tmp"
    printf '# BEGIN unifi-ap-bridge\n%s\n# END unifi-ap-bridge\n' "$body" >> "$tmp"
    chmod 755 "$tmp"; mv -f "$tmp" "$file"
}
upsert_hook "$SCRIPTS_DIR/services-start" "$INSTALL_DIR/start.sh </dev/null >/dev/null 2>&1 &"
upsert_hook "$SCRIPTS_DIR/services-stop" 'killall unifi-bridge 2>/dev/null || true'
upsert_hook "$SCRIPTS_DIR/service-event" 'if echo "$2" | /bin/grep -q "unifi_bridge"; then { /jffs/addons/unifi-bridge/apply_config.sh "$@" & }; fi'

echo "Installing verified UniFi AP Bridge $META_VERSION ($META_COMMIT), $ARCH"
"$INSTALL_DIR/start.sh"
echo "UniFi AP Bridge installed; WebUI slot: $USER_SLOT.asp"
