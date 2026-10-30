#!/bin/sh
# ==============================================================================
# UniFi AP Bridge - Cluster Node Synchronization & Status Tool
# ==============================================================================

ADDON_NAME="unifi-bridge"
INSTALL_DIR="/jffs/addons/$ADDON_NAME"
CONFIG_FILE="$INSTALL_DIR/config/config.json"
CLUSTER_JS="/tmp/var/wwwext/$ADDON_NAME/cluster.js"
CLUSTER_JSON="/tmp/$ADDON_NAME-cluster.json"
MAIN_IP=$(nvram get lan_ipaddr)
MAIN_NAME=$(nvram get lan_hostname)
[ -z "$MAIN_NAME" ] && MAIN_NAME=$(nvram get computer_name)
[ -z "$MAIN_NAME" ] && MAIN_NAME="ZenWiFi_XT8-Main"

mkdir -p "/tmp/var/wwwext/$ADDON_NAME"

# Locate SSH Cluster Identity Key
SSH_KEY=""
for k in "$INSTALL_DIR/.ssh/cluster_key" "/jffs/addons/mervlan/.ssh/vlan_manager" "/jffs/.ssh/dropbear_ed25519_host_key" "$INSTALL_DIR/.ssh/id_ed25519"; do
    if [ -f "$k" ]; then
        SSH_KEY="$k"
        break
    fi
done

# Dropbear on Merlin does not support OpenSSH ConnectTimeout. Bound the
# entire command (including connect/auth) with a watchdog; retain stdin for tar.
cluster_ssh() (
    # dash redirects stdin for background jobs before processing their redirects.
    # Duplicate the caller input first so transfers survive that redirection.
    exec 3<&0
    ssh -i "$SSH_KEY" -y -o BatchMode=yes -K 5 -I 15 "$@" <&3 3<&- &
    ssh_pid=$!
    (
        sleep "${CLUSTER_SSH_TIMEOUT:-30}" &
        sleep_pid=$!
        trap 'kill "$sleep_pid" 2>/dev/null; exit' TERM INT
        wait "$sleep_pid"
        kill "$ssh_pid" 2>/dev/null
    ) &
    timer_pid=$!
    trap 'kill "$ssh_pid" "$timer_pid" 2>/dev/null' TERM INT
    wait "$ssh_pid"
    result=$?
    kill "$timer_pid" 2>/dev/null
    wait "$timer_pid" 2>/dev/null
    exit "$result"
)

# Status freshness is measured on the node's own clock; no inter-node skew.
status_is_fresh() {
    case "$1:$2:$3" in *[!0-9:]*|:*|*::*|*:) return 1 ;; esac
    age=$(( $1 - $2 ))
    [ "$age" -ge 0 ] && [ "$age" -le "$3" ]
}

# Discover configured Satellite Nodes
get_node_ips() {
    # 1. From config.json "nodes" array if explicitly specified by user
    if [ -f "$CONFIG_FILE" ] && grep -q '"nodes"' "$CONFIG_FILE" 2>/dev/null; then
        cfg_ips=$(grep -A 10 '"nodes"' "$CONFIG_FILE" 2>/dev/null | grep -oE '([0-9]{1,3}\.){3}[0-9]{1,3}' | grep -v -x "$MAIN_IP" | sort -u)
        if [ -n "$cfg_ips" ]; then
            echo "$cfg_ips"
            return
        fi
    fi

    # 2. From native Asuswrt NVRAM (cfg_device_list & asus_device_list - standard AiMesh registries)
    nvram_ips=$( { nvram get cfg_device_list 2>/dev/null; nvram get asus_device_list 2>/dev/null; } | tr '<>' '\n\n' | grep -oE '([0-9]{1,3}\.){3}[0-9]{1,3}' | grep -v -x "$MAIN_IP" | grep -vE '(255\.255\.255|0\.0\.0\.0)' | sort -u )
    if [ -n "$nvram_ips" ]; then
        echo "$nvram_ips"
        return
    fi

    # 3. From MerVLAN settings.json if present
    if [ -f /jffs/addons/mervlan/settings/settings.json ]; then
        merv_ips=$(grep -E '"NODE[0-9]+"' /jffs/addons/mervlan/settings/settings.json 2>/dev/null | awk -F'"' '{print $4}' | grep -v -x "$MAIN_IP" | sort -u)
        if [ -n "$merv_ips" ]; then
            echo "$merv_ips"
            return
        fi
    fi
}

ACTION="${1:-sync}"
NOW=$(date +"%Y-%m-%d %H:%M:%S")
EXPLICIT_OPERATION_ID="${4:-}"
OPERATION_ID="${EXPLICIT_OPERATION_ID:-$(date +%s)$$}"
case "$OPERATION_ID" in ''|*[!0-9]*) OPERATION_ID=$(date +%s) ;; esac
[ "${#OPERATION_ID}" -le 24 ] || OPERATION_ID=$(date +%s)
case "$EXPLICIT_OPERATION_ID" in ''|*[!0-9]*|?????????????????????????*) EXPLICIT_OPERATION_ID="" ;; esac
MUTATION_LOCK="/tmp/unifi-bridge-mutation.lock"
mutation_lock_owned=0
operation_record_done=0
operation_failure_status="FAILED"
sync_tmp=""
OPERATION_RECORD_FILE=""
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
}
assert_no_update_journal() {
    [ ! -e "$INSTALL_DIR/.update-transaction" ] || {
        echo "Unfinished addon update journal requires recovery before cluster mutation" >&2; return 1;
    }
}
commits_match() {
    [ -n "$1" ] && [ -n "$2" ] || return 1
    [ "$1" = "$2" ] && return 0
    c1="$1"; c2="$2"
    # Require exact match if both are full (>= 40 chars) hashes.
    # Only permit short-prefix matching when at least one input is abbreviated (< 40 chars).
    if [ "${#c1}" -lt 40 ] || [ "${#c2}" -lt 40 ]; then
        if [ "${#c1}" -ge 7 ] && [ "${#c2}" -ge 7 ]; then
            p1=$(printf '%s' "$c1" | cut -c1-7)
            p2=$(printf '%s' "$c2" | cut -c1-7)
            [ "$p1" = "$p2" ] && return 0
        fi
    fi
    return 1
}

extract_json_field() {
    # 1. Try jq if installed and executable
    if which jq >/dev/null 2>&1 || [ -x /usr/bin/jq ]; then
        res=$(printf '%s' "$1" | jq -c "$2" 2>/dev/null)
        [ -n "$res" ] && [ "$res" != "null" ] && { printf '%s' "$res"; return 0; }
    fi
    # 2. Try unifi-bridge binary -extract-json helper if available
    field_name="${2#\.}"
    field_name="${field_name%% *}"
    field_name="${field_name%%\/\/*}"
    bridge_bin="/jffs/addons/unifi-bridge/unifi-bridge"
    if [ ! -x "$bridge_bin" ]; then
        bridge_bin=$(which unifi-bridge 2>/dev/null || true)
    fi
    if [ -x "$bridge_bin" ] && [ -n "$field_name" ]; then
        res=$(printf '%s' "$1" | "$bridge_bin" -extract-json "$field_name" 2>/dev/null)
        [ -n "$res" ] && [ "$res" != "null" ] && { printf '%s' "$res"; return 0; }
    fi
    # 3. If neither parser is available, distinguish missing parser from empty data
    if [ -n "$1" ] && [ "$1" != "{}" ]; then
        # Input has content but couldn't be parsed
        printf '%s' "$3"
        return 0
    fi
    printf '%s' "$3"
}

json_escape() {
    printf '%s' "$1" | tr -d '\r\n' | sed 's/\\/\\\\/g; s/"/\\"/g'
}
json_number_or_null() {
    case "$1" in ''|*[!0-9]*) printf 'null' ;; *) printf '%s' "$1" ;; esac
}
write_sync_operation() {
    [ -n "$EXPLICIT_OPERATION_ID" ] || return 0
    status="$1"; message="$2"; result_nodes="${3:-[]}"
    mkdir -p "/tmp/var/wwwext/$ADDON_NAME" || return 1
    OPERATION_RECORD_FILE="/tmp/var/wwwext/$ADDON_NAME/operation_${EXPLICIT_OPERATION_ID}.js"
    tmp="$OPERATION_RECORD_FILE.tmp.$$"
    completed_at=$(date +%s)
    message_json=$(json_escape "$message")
    printf 'window.unifiBridgeOperation = {"request_id":"%s","operation":"sync","status":"%s","completed_at":%s,"message":"%s","nodes":%s};\n' \
        "$EXPLICIT_OPERATION_ID" "$status" "$completed_at" "$message_json" "$result_nodes" > "$tmp" || return 1
    chmod 644 "$tmp" && mv -f "$tmp" "$OPERATION_RECORD_FILE" || return 1
    ls -1t "/tmp/var/wwwext/$ADDON_NAME"/operation_*.js 2>/dev/null | sed -n '21,$p' | while IFS= read -r old; do rm -f "$old"; done
}
sync_exit_trap() {
    exit_rc=$?
    release_mutation_lock
    rm -rf "${sync_tmp:-}"
    if [ "$ACTION" = "sync" ] && [ -n "$EXPLICIT_OPERATION_ID" ] && [ "$operation_record_done" -eq 0 ]; then
        write_sync_operation "$operation_failure_status" "Sync did not complete; inspect per-node status" "[]"
    fi
}
trap 'sync_exit_trap' 0
trap 'exit 130' INT
trap 'exit 143' TERM

if [ "$ACTION" = "sync" ] && [ -n "$EXPLICIT_OPERATION_ID" ]; then
    write_sync_operation RUNNING "Sync request accepted; per-node verification is pending" "[]"
fi


# Snapshot local inputs while holding only this node's mutation lock. Never
# retain it while waiting for a remote node; reciprocal syncs must not form a
# distributed lock cycle.
NODE_IPS=""
SYNC_CONFIG=""
REQUESTED_BRANCH="${2:-}"
REQUESTED_COMMIT="${3:-}"
if [ "$ACTION" = "sync" ] || [ "$ACTION" = "update" ]; then
    if ! acquire_mutation_lock; then operation_failure_status="REJECTED"; exit 1; fi
    if ! assert_no_update_journal; then operation_failure_status="REJECTED"; exit 1; fi
    sync_tmp="/tmp/unifi-bridge-sync-$$"
    (umask 077; mkdir "$sync_tmp") || exit 1
    NODE_IPS=$(get_node_ips)
    if [ "$ACTION" = "sync" ]; then
        [ -f "$CONFIG_FILE" ] && cp -p "$CONFIG_FILE" "$sync_tmp/config.json" || {
            echo "Main configuration snapshot unavailable" >&2; exit 1;
        }
        SYNC_CONFIG="$sync_tmp/config.json"
    else
        case "$REQUESTED_BRANCH" in main|dev) ;; *) echo "Invalid requested cluster release track" >&2; exit 1 ;; esac
        case "$REQUESTED_COMMIT" in *[!0-9a-f]*|'') REQUESTED_COMMIT="" ;; esac
        if [ -n "$REQUESTED_COMMIT" ] && [ "${#REQUESTED_COMMIT}" -ge 7 ] && [ "${#REQUESTED_COMMIT}" -le 40 ]; then
            :
        else
            REQUESTED_COMMIT=""
        fi
        [ -n "$REQUESTED_COMMIT" ] || { echo "Exact local source commit is unavailable" >&2; exit 1; }
    fi
    release_mutation_lock
    mutation_lock_owned=0
fi

# 1. Gather Main Node Local Status
main_status_js="/tmp/var/wwwext/$ADDON_NAME/status.js"
main_pid=$(pidof unifi-bridge 2>/dev/null | awk '{print $1}')
main_running="false"
main_commit=""
main_inform=""
main_freshness="unknown"
main_outcome="unknown"


main_total=0
main_2g=0
main_5g=0
main_cu_2g=0
main_cu_5g=0
main_model="UniFi 6 Mesh (U6M)"
main_mode="GCM"
main_heartbeat="-"
main_temp="-"

if [ -f "$main_status_js" ]; then
    main_total=$(grep '"total_stations":' "$main_status_js" | head -n 1 | awk -F':' '{print $2}' | tr -d ' \t\r\n,')
    main_2g=$(grep '"stations_2g":' "$main_status_js" | head -n 1 | awk -F':' '{print $2}' | tr -d ' \t\r\n,')
    main_5g=$(grep '"stations_5g":' "$main_status_js" | head -n 1 | awk -F':' '{print $2}' | tr -d ' \t\r\n,')
    main_cu_2g=$(grep '"cu_total_2g":' "$main_status_js" | head -n 1 | awk -F':' '{print $2}' | tr -d ' \t\r\n,')
    main_cu_5g=$(grep '"cu_total_5g":' "$main_status_js" | head -n 1 | awk -F':' '{print $2}' | tr -d ' \t\r\n,')
    m_val=$(grep '"model":' "$main_status_js" | head -n 1 | awk -F'"' '{print $4}')
    [ -n "$m_val" ] && main_model="$m_val"
    m_mode=$(grep '"mode":' "$main_status_js" | head -n 1 | awk -F'"' '{print $4}')
    [ -n "$m_mode" ] && main_mode="$m_mode"
    m_hb=$(grep '"last_heartbeat":' "$main_status_js" | head -n 1 | awk -F'"' '{print $4}')
    [ -n "$m_hb" ] && main_heartbeat="$m_hb"
fi

main_updated=$(grep '"status_updated_at":' "$main_status_js" 2>/dev/null | head -n 1 | sed -n 's/.*"status_updated_at":[[:space:]]*\([0-9][0-9]*\).*/\1/p')
main_commit=$(sed -n 's/.*"commit":[[:space:]]*"\([^"]*\)".*/\1/p' "$main_status_js" 2>/dev/null | head -n 1)
main_inform=$(sed -n 's/.*"last_successful_inform":[[:space:]]*"\([^"]*\)".*/\1/p' "$main_status_js" 2>/dev/null | head -n 1)
case "$main_commit" in *[!0-9a-f]*|'') main_commit="" ;; esac
if [ -n "$main_commit" ] && [ "${#main_commit}" -ge 7 ] && [ "${#main_commit}" -le 40 ]; then
    :
else
    main_commit=""
fi
main_interval=$(grep '"interval_sec":' "$main_status_js" 2>/dev/null | head -n 1 | awk -F':' '{print $2}' | tr -d ' \t\r\n,')
case "$main_interval" in ''|*[!0-9]*) main_interval=10 ;; esac
if status_is_fresh "$(date +%s)" "$main_updated" "$((main_interval * 3 + 30))"; then
    main_freshness="fresh"
    if [ -n "$main_pid" ] && [ "$(grep '"pid":' "$main_status_js" 2>/dev/null | head -n 1 | awk -F':' '{print $2}' | tr -d ' \t\r\n,')" = "$main_pid" ]; then
        main_running="true"
    fi
else
    main_freshness="stale"
fi
main_version=$(sed -n 's/.*"version":[[:space:]]*"\([^"]*\)".*/\1/p' "$main_status_js" 2>/dev/null | head -n 1)
[ -n "$main_version" ] || main_version=$(cat "$INSTALL_DIR/version.txt" 2>/dev/null || echo "unknown")
main_enabled="false"
printf '%s' "$(cat "$main_status_js" 2>/dev/null)" | grep -Eq '"enabled"[[:space:]]*:[[:space:]]*true' && main_enabled="true"
main_last_status=$(sed -n 's/.*"last_status":[[:space:]]*"\([^"]*\)".*/\1/p' "$main_status_js" 2>/dev/null | head -n 1)
if [ "$main_freshness" = "fresh" ] && {
    { [ "$main_enabled" = "false" ] && [ -z "$main_pid" ]; } ||
    { [ "$main_enabled" = "true" ] && [ "$main_running" = "true" ] && [ -n "$main_inform" ] && case "$main_last_status" in Active\ *) true ;; *) false ;; esac; }
}; then
    main_outcome="success"
fi

if [ -f "/sys/class/thermal/thermal_zone0/temp" ]; then
    raw_temp=$(cat /sys/class/thermal/thermal_zone0/temp 2>/dev/null)
    [ -n "$raw_temp" ] && [ "$raw_temp" -gt 0 ] && main_temp=$(awk "BEGIN {printf \"%.1f °C\", $raw_temp / 1000}")
fi

# Extract hardware ports, radios, and system stats
main_raw_json=$(sed -e '1s/^[^{]*{/{/' -e '$s/};[[:space:]]*$/}/' "$main_status_js" 2>/dev/null)
main_ports=$(extract_json_field "$main_raw_json" '.ports // []' '[]')
main_radios=$(extract_json_field "$main_raw_json" '.radio_stats // []' '[]')
main_system=$(extract_json_field "$main_raw_json" '.system_stats // {}' '{}')
main_stage_errors=$(extract_json_field "$main_raw_json" '.sample_stage_errors // {}' '{}')
[ -n "$main_ports" ] || main_ports="[]"
[ -n "$main_radios" ] || main_radios="[]"
[ -n "$main_system" ] || main_system="{}"
[ -n "$main_stage_errors" ] || main_stage_errors="{}"

# Initialize JSON output
main_version_json=$(json_escape "$main_version")
main_commit_json=$( [ -n "$main_commit" ] && printf '"%s"' "$main_commit" || printf 'null' )
main_updated_json=$(json_number_or_null "$main_updated")
main_inform_json=$( [ -n "$main_inform" ] && printf '"%s"' "$(json_escape "$main_inform")" || printf 'null' )
main_model_json=$(json_escape "$main_model")
main_mode_json=$(json_escape "$main_mode")
main_temp_json=$(json_escape "$main_temp")
main_hb_json=$(json_escape "$main_heartbeat")
main_name_json=$(json_escape "$MAIN_NAME")
main_ip_json=$(json_escape "$MAIN_IP")
main_pid_json=$( [ -n "$main_pid" ] && json_escape "$main_pid" | sed 's/.*/"&"/' || printf 'null' )
nodes_json="["
nodes_json="${nodes_json}
    {
      \"name\": \"${main_name_json}\",
      \"ip\": \"${main_ip_json}\",
      \"role\": \"Main Gateway\",
      \"model\": \"${main_model_json}\",
      \"version\": \"${main_version_json}\",
      \"commit\": ${main_commit_json},
      \"status\": \"$( [ "$main_running" = "true" ] && echo "RUNNING" || echo "STOPPED" )\",
      \"pid\": ${main_pid_json},
      \"enabled\": ${main_enabled},
      \"running\": ${main_running},
      \"status_updated_at\": ${main_updated_json},
      \"freshness\": \"${main_freshness}\",
      \"last_successful_inform\": ${main_inform_json},
      \"operation_id\": \"${OPERATION_ID}\",
      \"requested_version\": null,
      \"requested_commit\": ${main_commit_json},
      \"dispatch_result\": \"not_applicable\",
      \"stage\": \"observed\",
      \"outcome\": \"${main_outcome}\",
      \"total_clients\": ${main_total:-0},
      \"clients_2g\": ${main_2g:-0},
      \"clients_5g\": ${main_5g:-0},
      \"cu_2g\": ${main_cu_2g:-0},
      \"cu_5g\": ${main_cu_5g:-0},
      \"mode\": \"${main_mode_json}\",
      \"temp\": \"${main_temp_json}\",
      \"last_heartbeat\": \"${main_hb_json}\",
      \"sync_status\": \"Local\",
      \"ports\": ${main_ports},
      \"radio_stats\": ${main_radios},
      \"system_stats\": ${main_system},
      \"stage_errors\": ${main_stage_errors}
    }"

# 2. Iterate Satellite Nodes
[ "$ACTION" = "sync" ] || [ "$ACTION" = "update" ] || NODE_IPS=$(get_node_ips)

for node_ip in $NODE_IPS; do
    [ -z "$node_ip" ] && continue
    sync_status="In Sync"
    [ "$ACTION" = "status" ] || sync_status="Not Verified"
    node_running="false"
    node_name="ZenWiFi_Node_${node_ip##*.}"
    node_model="unknown"
    node_version="unknown"
    node_commit=""
    node_ports="[]"
    node_radios="[]"
    node_system="{}"
    node_stage_errors="{}"
    node_updated=""
    node_inform=""
    node_freshness="unknown"
    node_stage="not_dispatched"
    node_outcome="unknown"
    dispatch_result="not_attempted"
    remote_operation_status=""
    remote_operation_id=""
    remote_operation_name=""
    remote_operation_version=""
    remote_operation_commit=""
    node_health_verified="false"
    node_pid="-"
    r_enabled=""
    r_run=""
    node_total=0
    node_2g=0
    node_5g=0
    node_cu_2g=0
    node_cu_5g=0
    node_mode="-"
    node_temp="-"
    node_hb="-"

    if [ -z "$SSH_KEY" ]; then
        sync_status="No SSH Key"
    else
        if [ "$ACTION" = "update" ]; then
            echo "[*] Requesting exact-commit updater on node ${node_ip}..."
            node_stage="dispatched"
            CLUSTER_SSH_TIMEOUT=240 cluster_ssh "$node_ip" "$INSTALL_DIR/update.sh apply-exact $REQUESTED_BRANCH $REQUESTED_COMMIT $OPERATION_ID" >/dev/null 2>&1
            [ "$?" -eq 0 ] && dispatch_result="succeeded" || dispatch_result="unknown"
        elif [ "$ACTION" = "sync" ]; then
            echo "[*] Synchronizing shared settings to node ${node_ip}..."
            node_stage="dispatched"
            remote_sync_cmd=$(cat <<'REMOTE'
umask 077
MUTATION_LOCK=/tmp/unifi-bridge-mutation.lock
operation_id=@OPERATION_ID@
operation_done=0
write_operation() {
    printf '{"operation_id":"%s","operation":"sync","status":"%s"}\n' "$operation_id" "$1" > /tmp/var/wwwext/unifi-bridge/cluster_operation.js.tmp && mv -f /tmp/var/wwwext/unifi-bridge/cluster_operation.js.tmp /tmp/var/wwwext/unifi-bridge/cluster_operation.js
}
if ! mkdir "$MUTATION_LOCK" 2>/dev/null; then exit 75; fi
trap 'rc=$?; if [ "$operation_done" -ne 1 ]; then write_operation FAILED; fi; rm -f "$MUTATION_LOCK/pid"; rmdir "$MUTATION_LOCK" 2>/dev/null || true' 0
printf "%s\n" "$$" > "$MUTATION_LOCK/pid" || exit 76
[ ! -e /jffs/addons/unifi-bridge/.update-transaction ] || exit 77
mkdir -p /jffs/addons/unifi-bridge/config /jffs/addons/unifi-bridge/.ssh /tmp/var/wwwext/unifi-bridge || exit 78
chmod 755 /jffs/addons/unifi-bridge; chmod 700 /jffs/addons/unifi-bridge/.ssh
BRIDGE_BIN=/tmp/unifi-bridge/unifi-bridge
[ -x "$BRIDGE_BIN" ] || BRIDGE_BIN=/jffs/addons/unifi-bridge/bin/unifi-bridge
[ -x "$BRIDGE_BIN" ] || exit 79
UNIFI_BRIDGE_MUTATION_LOCK_HELD=1 UNIFI_BRIDGE_MUTATION_LOCK_OWNER="$$" "$BRIDGE_BIN" -config /jffs/addons/unifi-bridge/config/config.json -sync-settings -gateway "$(nvram get lan_gateway)" || exit 80
UNIFI_BRIDGE_MUTATION_LOCK_OWNER="$$" /jffs/addons/unifi-bridge/start.sh || exit 81
write_operation APPLIED || exit 82
operation_done=1
REMOTE
)
            remote_sync_cmd=$(printf '%s' "$remote_sync_cmd" | sed "s/@OPERATION_ID@/$OPERATION_ID/g")
            cat "$SYNC_CONFIG" | CLUSTER_SSH_TIMEOUT=90 cluster_ssh "$node_ip" "$remote_sync_cmd" >/dev/null 2>&1
            [ "$?" -eq 0 ] && dispatch_result="succeeded" || dispatch_result="unknown"
        fi

        # Read runtime and operation state after dispatch. A lost SSH reply is
        # unknown until a matching operation ID and fresh runtime identity prove
        # that the remote action completed.
        remote_raw=$(cluster_ssh "$node_ip" "cat /tmp/var/wwwext/$ADDON_NAME/status.js 2>/dev/null; echo '---LIVE---'; date +%s; pidof unifi-bridge; p=\$(pidof unifi-bridge 2>/dev/null | awk '{print \$1}'); [ -n \"\$p\" ] && readlink /proc/\$p/exe 2>/dev/null; echo '---VER---'; cat $INSTALL_DIR/version.txt 2>/dev/null; echo '---OP---'; cat /tmp/var/wwwext/$ADDON_NAME/cluster_operation.js 2>/dev/null; echo '---UPDATE---'; cat /tmp/var/wwwext/$ADDON_NAME/update_status.js 2>/dev/null; echo '---TEMP---'; cat /sys/class/thermal/thermal_zone0/temp 2>/dev/null" </dev/null 2>/dev/null)
        remote_result=$?
        if [ "$remote_result" -eq 0 ] && [ -n "$remote_raw" ]; then
            remote_status=$(printf '%s\n' "$remote_raw" | sed -n '1,/---LIVE---/p' | sed '$d')
            remote_op=$(printf '%s\n' "$remote_raw" | sed -n '/---OP---/,/---UPDATE---/p' | sed '1d;$d')
            remote_update=$(printf '%s\n' "$remote_raw" | sed -n '/---UPDATE---/,/---TEMP---/p' | sed '1d;$d')
            r_host=$(printf '%s' "$remote_status" | grep '"hostname":' | head -n 1 | awk -F'"' '{print $4}')
            [ -n "$r_host" ] && node_name="$r_host"
            r_now=$(printf '%s\n' "$remote_raw" | sed -n '/---LIVE---/{n;p;}')
            r_live_pid=$(printf '%s\n' "$remote_raw" | sed -n '/---LIVE---/{n;n;p;}')
            r_exe=$(printf '%s\n' "$remote_raw" | sed -n '/---LIVE---/{n;n;n;p;}')
            r_updated=$(printf '%s' "$remote_status" | sed -n 's/.*"status_updated_at":[[:space:]]*\([0-9][0-9]*\).*/\1/p' | head -n 1)
            r_interval=$(printf '%s' "$remote_status" | sed -n 's/.*"interval_sec":[[:space:]]*\([0-9][0-9]*\).*/\1/p' | head -n 1)
            case "$r_interval" in ''|*[!0-9]*) r_interval=10 ;; esac
            max_age=$((r_interval * 3 + 30))
            r_run="false"
            r_enabled="false"
            printf '%s' "$remote_status" | grep -Eq '"running"[[:space:]]*:[[:space:]]*true' && r_run="true"
            printf '%s' "$remote_status" | grep -Eq '"enabled"[[:space:]]*:[[:space:]]*true' && r_enabled="true"
            node_version=$(printf '%s' "$remote_status" | sed -n 's/.*"version":[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)
            node_model=$(printf '%s' "$remote_status" | sed -n 's/.*"model":[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)
            [ -n "$node_model" ] || node_model="unknown"
            node_commit=$(printf '%s' "$remote_status" | sed -n 's/.*"commit":[[:space:]]*"\([0-9a-f]*\)".*/\1/p' | head -n 1)
            node_pid=$(printf '%s' "$remote_status" | sed -n 's/.*"pid":[[:space:]]*\([0-9][0-9]*\).*/\1/p' | head -n 1)
            node_updated="$r_updated"
            node_inform=$(printf '%s' "$remote_status" | sed -n 's/.*"last_successful_inform":[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)
            node_last_status=$(printf '%s' "$remote_status" | sed -n 's/.*"last_status":[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)
            [ -n "$node_version" ] || node_version="unknown"
            if status_is_fresh "$r_now" "$r_updated" "$max_age"; then
                node_freshness="fresh"
            else
                node_freshness="stale"
                sync_status="Stale or Stopped"
            fi
            if [ "$r_run" = "true" ] && [ -n "$r_live_pid" ] && [ "$node_pid" = "$r_live_pid" ] &&
               [ "$r_exe" = "/tmp/unifi-bridge/unifi-bridge" ] && [ "$node_freshness" = "fresh" ] &&
               [ -n "$node_inform" ] && case "$node_last_status" in Active\ *) true ;; *) false ;; esac; then
                node_running="true"
                node_health_verified="true"
            elif [ "$r_run" = "false" ] && [ "$r_enabled" = "false" ] && [ -z "$r_live_pid" ] && [ "$node_freshness" = "fresh" ]; then
                node_running="false"
                node_health_verified="true"
            else
                sync_status="Stale or Stopped"
            fi
            r_tot=$(printf '%s' "$remote_status" | sed -n 's/.*"total_stations":[[:space:]]*\([0-9][0-9]*\).*/\1/p' | head -n 1)
            [ -n "$r_tot" ] && node_total="$r_tot"
            r_2g=$(printf '%s' "$remote_status" | sed -n 's/.*"stations_2g":[[:space:]]*\([0-9][0-9]*\).*/\1/p' | head -n 1)
            [ -n "$r_2g" ] && node_2g="$r_2g"
            r_5g=$(printf '%s' "$remote_status" | sed -n 's/.*"stations_5g":[[:space:]]*\([0-9][0-9]*\).*/\1/p' | head -n 1)
            [ -n "$r_5g" ] && node_5g="$r_5g"
            r_cu2=$(printf '%s' "$remote_status" | sed -n 's/.*"cu_total_2g":[[:space:]]*\([0-9][0-9]*\).*/\1/p' | head -n 1)
            [ -n "$r_cu2" ] && node_cu_2g="$r_cu2"
            r_cu5=$(printf '%s' "$remote_status" | sed -n 's/.*"cu_total_5g":[[:space:]]*\([0-9][0-9]*\).*/\1/p' | head -n 1)
            [ -n "$r_cu5" ] && node_cu_5g="$r_cu5"
            node_mode=$(printf '%s' "$remote_status" | sed -n 's/.*"mode":[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)
            node_hb=$(printf '%s' "$remote_status" | sed -n 's/.*"last_heartbeat":[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)
            r_temp_raw=$(printf '%s\n' "$remote_raw" | sed -n '/---TEMP---/,$p' | grep -oE '^[0-9]+$')
            if [ -n "$r_temp_raw" ] && [ "$r_temp_raw" -gt 0 ]; then
                node_temp=$(awk "BEGIN {printf \"%.1f °C\", $r_temp_raw / 1000}")
            fi
            case "$node_commit" in *[!0-9a-f]*|'') node_commit="" ;; esac
            if [ -n "$node_commit" ] && [ "${#node_commit}" -ge 7 ] && [ "${#node_commit}" -le 40 ]; then
                :
            else
                node_commit=""
            fi
            remote_operation_id=$(printf '%s' "$remote_op" | sed -n 's/.*"operation_id":[[:space:]]*"\([0-9]*\)".*/\1/p' | head -n 1)
            remote_operation_status=$(printf '%s' "$remote_op" | sed -n 's/.*"status":[[:space:]]*"\([A-Z_]*\)".*/\1/p' | head -n 1)
            remote_operation_name=$(printf '%s' "$remote_op" | sed -n 's/.*"operation":[[:space:]]*"\([a-z_]*\)".*/\1/p' | head -n 1)
            remote_operation_version=$(printf '%s' "$remote_op" | sed -n 's/.*"version":[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)
            remote_operation_commit=$(printf '%s' "$remote_op" | sed -n 's/.*"commit":[[:space:]]*"\([0-9a-f]*\)".*/\1/p' | head -n 1)
            remote_update_id=$(printf '%s' "$remote_update" | sed -n 's/.*"request_id":[[:space:]]*"\([0-9]*\)".*/\1/p' | head -n 1)
            remote_update_status=$(printf '%s' "$remote_update" | sed -n 's/.*"status":[[:space:]]*"\([A-Z_]*\)".*/\1/p' | head -n 1)
            node_raw_json=$(printf '%s\n' "$remote_status" | sed -e '1s/^[^{]*{/{/' -e '$s/};[[:space:]]*$/}/')
            node_ports=$(extract_json_field "$node_raw_json" '.ports // []' '[]')
            node_radios=$(extract_json_field "$node_raw_json" '.radio_stats // []' '[]')
            node_system=$(extract_json_field "$node_raw_json" '.system_stats // {}' '{}')
            node_stage_errors=$(extract_json_field "$node_raw_json" '.sample_stage_errors // {}' '{}')
            [ -n "$node_ports" ] || node_ports="[]"
            [ -n "$node_radios" ] || node_radios="[]"
            [ -n "$node_system" ] || node_system="{}"
            [ -n "$node_stage_errors" ] || node_stage_errors="{}"
            if [ "$ACTION" = "update" ]; then
                node_stage="verification_pending"
                if [ "$remote_operation_id" = "$OPERATION_ID" ] && [ "$remote_operation_name" = "update" ] && [ "$remote_operation_status" = "FAILED" ]; then
                    node_stage="failed"; node_outcome="failure"; sync_status="Update Failed"
                elif [ "$remote_operation_id" = "$OPERATION_ID" ] && [ "$remote_operation_name" = "update" ] &&
                     [ "$remote_operation_status" = "APPLIED" ] && [ "$node_health_verified" = "true" ] &&
                     commits_match "$node_commit" "$REQUESTED_COMMIT"; then
                    node_stage="verified"; node_outcome="success"; sync_status="Update Verified"
                elif [ "$remote_update_id" = "$OPERATION_ID" ] && [ "$remote_update_status" = "ERROR" ]; then
                    node_stage="failed"; node_outcome="failure"; sync_status="Update Failed"
                elif [ "$remote_update_id" = "$OPERATION_ID" ] && [ "$remote_update_status" = "SUCCESS" ] &&
                     [ "$node_health_verified" = "true" ] && commits_match "$node_commit" "$REQUESTED_COMMIT"; then
                    node_stage="verified"; node_outcome="success"; sync_status="Update Verified"
                fi
            elif [ "$ACTION" = "sync" ]; then
                node_stage="verification_pending"
                if [ "$remote_operation_id" = "$OPERATION_ID" ] && [ "$remote_operation_name" = "sync" ] && [ "$remote_operation_status" = "FAILED" ]; then
                    node_stage="failed"; node_outcome="failure"; sync_status="Config Apply Failed"
                elif [ "$remote_operation_id" = "$OPERATION_ID" ] && [ "$remote_operation_name" = "sync" ] && [ "$remote_operation_status" = "APPLIED" ] &&
                   [ "$node_health_verified" = "true" ] && commits_match "$node_commit" "$main_commit"; then
                    node_stage="verified"; node_outcome="success"; sync_status="In Sync"
                fi
            else
                node_stage="observed"
                if [ "$node_health_verified" = "true" ] && [ "$node_commit" != "" ]; then
                    node_outcome="success"
                    if [ "$node_version" = "$main_version" ]; then
                        if [ -n "$main_commit" ] && [ -n "$node_commit" ]; then
                            if commits_match "$node_commit" "$main_commit"; then
                                sync_status="In Sync"
                            else
                                sync_status="Version Mismatch"
                            fi
                        else
                            sync_status="In Sync"
                        fi
                    else
                        sync_status="Version Mismatch"
                    fi
                else
                    node_outcome="unknown"; sync_status="Stale or Stopped"
                fi
            fi
        else
            node_stage="unreachable"
            node_freshness="unknown"
            node_outcome="unknown"
            sync_status="Unreachable"
        fi
    fi

    node_name_json=$(json_escape "$node_name")
    node_ip_json=$(json_escape "$node_ip")
    node_model_json=$(json_escape "$node_model")
    node_version_json=$(json_escape "$node_version")
    node_commit_json=$( [ -n "$node_commit" ] && printf '"%s"' "$node_commit" || printf 'null' )
    node_updated_json=$(json_number_or_null "$node_updated")
    node_inform_json=$( [ -n "$node_inform" ] && printf '"%s"' "$(json_escape "$node_inform")" || printf 'null' )
    node_pid_json=$( [ -n "$node_pid" ] && [ "$node_pid" != "-" ] && json_escape "$node_pid" | sed 's/.*/"&"/' || printf 'null' )
    node_mode_json=$(json_escape "$node_mode")
    node_temp_json=$(json_escape "$node_temp")
    node_hb_json=$(json_escape "$node_hb")
    sync_status_json=$(json_escape "$sync_status")
    requested_commit_json=$( [ "$ACTION" = "update" ] && printf '"%s"' "$REQUESTED_COMMIT" || { [ -n "$main_commit" ] && printf '"%s"' "$main_commit" || printf 'null'; } )
    requested_version_json=$( [ "$ACTION" = "update" ] && printf '"%s"' "$(json_escape "$main_version")" || printf 'null' )
    node_enabled_json=$( [ -n "${r_enabled:-}" ] && printf '%s' "$r_enabled" || printf 'null' )
    node_running_json=$( [ -n "${r_run:-}" ] && printf '%s' "$r_run" || printf 'null' )
    dispatch_result_json=$(json_escape "$dispatch_result")
    last_operation_id_json=$( [ -n "$remote_operation_id" ] && printf '"%s"' "$remote_operation_id" || printf 'null' )
    last_operation_name_json=$( [ -n "$remote_operation_name" ] && printf '"%s"' "$(json_escape "$remote_operation_name")" || printf 'null' )
    last_operation_status_json=$( [ -n "$remote_operation_status" ] && printf '"%s"' "$remote_operation_status" || printf 'null' )
    last_operation_version_json=$( [ -n "$remote_operation_version" ] && printf '"%s"' "$(json_escape "$remote_operation_version")" || printf 'null' )
    last_operation_commit_json=$( [ -n "$remote_operation_commit" ] && printf '"%s"' "$remote_operation_commit" || printf 'null' )
    nodes_json="${nodes_json},
    {
      \"name\": \"${node_name_json}\",
      \"ip\": \"${node_ip_json}\",
      \"role\": \"Satellite Node\",
      \"model\": \"${node_model_json}\",
      \"version\": \"${node_version_json}\",
      \"commit\": ${node_commit_json},
      \"status\": \"$( [ "$node_running" = "true" ] && echo "RUNNING" || echo "STOPPED" )\",
      \"pid\": ${node_pid_json},
      \"enabled\": ${node_enabled_json},
      \"running\": ${node_running_json},
      \"status_updated_at\": ${node_updated_json},
      \"freshness\": \"${node_freshness}\",
      \"last_successful_inform\": ${node_inform_json},
      \"operation_id\": \"${OPERATION_ID}\",
      \"last_operation_id\": ${last_operation_id_json},
      \"last_operation\": ${last_operation_name_json},
      \"last_operation_status\": ${last_operation_status_json},
      \"last_operation_version\": ${last_operation_version_json},
      \"last_operation_commit\": ${last_operation_commit_json},
      \"requested_version\": ${requested_version_json},
      \"requested_commit\": ${requested_commit_json},
      \"dispatch_result\": \"${dispatch_result_json}\",
      \"stage\": \"${node_stage}\",
      \"outcome\": \"${node_outcome}\",
      \"total_clients\": ${node_total:-0},
      \"clients_2g\": ${node_2g:-0},
      \"clients_5g\": ${node_5g:-0},
      \"cu_2g\": ${node_cu_2g:-0},
      \"cu_5g\": ${node_cu_5g:-0},
      \"mode\": \"${node_mode_json}\",
      \"temp\": \"${node_temp_json}\",
      \"last_heartbeat\": \"${node_hb_json}\",
      \"sync_status\": \"${sync_status_json}\",
      \"ports\": ${node_ports},
      \"radio_stats\": ${node_radios},
      \"system_stats\": ${node_system},
      \"stage_errors\": ${node_stage_errors}
    }"
done

nodes_json="${nodes_json}
  ]"

# Write cluster.js atomically for WebUI consumption
cluster_tmp="$CLUSTER_JS.tmp.$$"
cat << EOF > "$cluster_tmp"
window.unifiBridgeCluster = {
  "last_sync": "${NOW}",
  "nodes": ${nodes_json}
};
EOF

chmod 644 "$cluster_tmp" && mv -f "$cluster_tmp" "$CLUSTER_JS" || exit 1
if [ "$ACTION" = "sync" ] && [ -n "$EXPLICIT_OPERATION_ID" ]; then
    failure_count=$(printf '%s' "$nodes_json" | grep -o '"outcome":[[:space:]]*"failure"' | wc -l | tr -d ' ')
    unknown_count=$(printf '%s' "$nodes_json" | grep -o '"outcome":[[:space:]]*"unknown"' | wc -l | tr -d ' ')
    case "$failure_count:$unknown_count" in *[!0-9:]*) failure_count=0; unknown_count=0 ;; esac
    if [ "$failure_count" -gt 0 ]; then
        result_state="PARTIAL"
    elif [ "$unknown_count" -gt 0 ]; then
        result_state="UNKNOWN"
    else
        result_state="COMPLETED"
    fi
    write_sync_operation "$result_state" "Sync completed with ${failure_count} failed and ${unknown_count} unverified nodes" "$nodes_json" || exit 1
    operation_record_done=1
fi
echo "Cluster sync completed at ${NOW}"
