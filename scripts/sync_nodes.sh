#!/bin/sh
# ==============================================================================
# UniFi AP Bridge - Cluster Node Synchronization & Status Tool
# ==============================================================================

ADDON_NAME="unifi-bridge"
INSTALL_DIR="/jffs/addons/$ADDON_NAME"
# Protect existing cluster credentials as well as newly provisioned directories.
mkdir -p "$INSTALL_DIR/.ssh"
# Status runs periodically: avoid chmod writes when modes are already correct.
[ "$(ls -ld "$INSTALL_DIR" | awk '{print $1}')" = "drwxr-xr-x" ] || chmod 755 "$INSTALL_DIR"
[ "$(ls -ld "$INSTALL_DIR/.ssh" | awk '{print $1}')" = "drwx------" ] || chmod 700 "$INSTALL_DIR/.ssh"

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

# 1. Gather Main Node Local Status
main_status_js="/tmp/var/wwwext/$ADDON_NAME/status.js"
main_pid=$(pidof unifi-bridge 2>/dev/null | awk '{print $1}')
main_running="false"
[ -n "$main_pid" ] && main_running="true"

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

main_updated=$(grep '"status_updated_at":' "$main_status_js" 2>/dev/null | head -n 1 | awk -F':' '{print $2}' | tr -d ' \t\r\n,')
main_interval=$(grep '"interval_sec":' "$main_status_js" 2>/dev/null | head -n 1 | awk -F':' '{print $2}' | tr -d ' \t\r\n,')
case "$main_interval" in ''|*[!0-9]*) main_interval=10 ;; esac
if ! status_is_fresh "$(date +%s)" "$main_updated" "$((main_interval * 3 + 30))"; then
    main_running="false"
fi

main_version=$(grep '"version":' "$main_status_js" 2>/dev/null | head -n 1 | awk -F'"' '{print $4}')
[ -n "$main_version" ] || main_version=$(cat "$INSTALL_DIR/version.txt" 2>/dev/null || echo "unknown")

if [ -f "/sys/class/thermal/thermal_zone0/temp" ]; then
    raw_temp=$(cat /sys/class/thermal/thermal_zone0/temp 2>/dev/null)
    [ -n "$raw_temp" ] && [ "$raw_temp" -gt 0 ] && main_temp=$(awk "BEGIN {printf \"%.1f °C\", $raw_temp / 1000}")
fi

# Initialize JSON output
nodes_json="["
nodes_json="${nodes_json}
    {
      \"name\": \"${MAIN_NAME}\",
      \"ip\": \"${MAIN_IP}\",
      \"role\": \"Main Gateway\",
      \"model\": \"${main_model}\",
      \"version\": \"${main_version}\",
      \"status\": \"$( [ "$main_running" = "true" ] && echo "RUNNING" || echo "STOPPED" )\",
      \"pid\": \"${main_pid:--}\",
      \"total_clients\": ${main_total:-0},
      \"clients_2g\": ${main_2g:-0},
      \"clients_5g\": ${main_5g:-0},
      \"cu_2g\": ${main_cu_2g:-0},
      \"cu_5g\": ${main_cu_5g:-0},
      \"mode\": \"${main_mode}\",
      \"temp\": \"${main_temp}\",
      \"last_heartbeat\": \"${main_heartbeat}\",
      \"sync_status\": \"Local\"
    }"

# 2. Iterate Satellite Nodes
NODE_IPS=$(get_node_ips)

for node_ip in $NODE_IPS; do
    [ -z "$node_ip" ] && continue
    sync_status="Version Match (Config Unverified)"
    node_running="false"
    node_name="ZenWiFi_Node_${node_ip##*.}"
    node_model="$main_model"
    node_version="$main_version"
    node_pid="-"
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
        # If action is 'update', push updated scripts, webui, version and binary to remote node
        if [ "$ACTION" = "update" ]; then
            echo "[*] Propagating update to node ${node_ip}..."
            # Stream updated scripts and binary package (excluding node-unique state.json)
            tar -czf - -C "$INSTALL_DIR" start.sh apply_config.sh sync_nodes.sh unifi-bridge.sh update.sh watchdog.sh version.txt webui config/config.json unifi-bridge.tar.gz 2>/dev/null | \
                CLUSTER_SSH_TIMEOUT=120 cluster_ssh "$node_ip" "tar -xzf - -C $INSTALL_DIR 2>/dev/null; [ -f $INSTALL_DIR/config/config.json ] && sed -i 's/\"uplink_remote_port\": [0-9]*/\"uplink_remote_port\": 0/' $INSTALL_DIR/config/config.json; [ -f $INSTALL_DIR/unifi-bridge.sh ] && ln -sf $INSTALL_DIR/unifi-bridge.sh /jffs/scripts/unifi-bridge; chmod 755 $INSTALL_DIR/*.sh; mkdir -p $INSTALL_DIR/.ssh; chmod 755 $INSTALL_DIR; chmod 700 $INSTALL_DIR/.ssh; $INSTALL_DIR/start.sh" >/dev/null 2>&1
            if [ $? -eq 0 ]; then
                sync_status="Update Applied (Unverified)"
            else
                sync_status="Update Failed"
            fi
        # If action is 'sync', push local config to remote node
        elif [ "$ACTION" = "sync" ] && [ -f "$CONFIG_FILE" ]; then
            # Remote config with node-specific state path and dynamic LLDP remote port
            sed -e 's/"uplink_remote_port": [0-9]*/"uplink_remote_port": 0/' "$CONFIG_FILE" | \
                CLUSTER_SSH_TIMEOUT=120 cluster_ssh "$node_ip" "mkdir -p $INSTALL_DIR/config $INSTALL_DIR/.ssh; chmod 755 $INSTALL_DIR; chmod 700 $INSTALL_DIR/.ssh; umask 077; cat > $INSTALL_DIR/config/config.json && chmod 600 $INSTALL_DIR/config/config.json && $INSTALL_DIR/start.sh" >/dev/null 2>&1
            if [ $? -eq 0 ]; then
                sync_status="Config Synced"
            else
                sync_status="Sync Failed"
            fi
        fi

        # Query remote node status
        remote_raw=$(cluster_ssh "$node_ip" "cat /tmp/var/wwwext/$ADDON_NAME/status.js 2>/dev/null; echo '---LIVE---'; date +%s; pidof unifi-bridge; echo '---VER---'; cat $INSTALL_DIR/version.txt 2>/dev/null; echo '---TEMP---'; cat /sys/class/thermal/thermal_zone0/temp 2>/dev/null" </dev/null 2>/dev/null)
        remote_result=$?
        if [ "$remote_result" -eq 0 ] && [ -n "$remote_raw" ]; then
            r_host=$(echo "$remote_raw" | grep '"hostname":' | head -n 1 | awk -F'"' '{print $4}')
            [ -n "$r_host" ] && node_name="$r_host"

            r_now=$(echo "$remote_raw" | sed -n '/---LIVE---/{n;p;}')
            r_live_pid=$(echo "$remote_raw" | sed -n '/---LIVE---/{n;n;p;}')
            r_updated=$(echo "$remote_raw" | grep '"status_updated_at":' | head -n 1 | awk -F':' '{print $2}' | tr -d ' \t\r\n,')
            r_interval=$(echo "$remote_raw" | grep '"interval_sec":' | head -n 1 | awk -F':' '{print $2}' | tr -d ' \t\r\n,')
            case "$r_interval" in ''|*[!0-9]*) r_interval=10 ;; esac
            max_age=$((r_interval * 3 + 30))
            r_run=$(echo "$remote_raw" | grep '"running":' | head -n 1 | awk -F':' '{print $2}' | tr -d ' \t\r\n,')
            if [ "$r_run" = "true" ] && [ -n "$r_live_pid" ] && status_is_fresh "$r_now" "$r_updated" "$max_age"; then
                node_running="true"
            else
                sync_status="Stale or Stopped"
            fi

            r_pid=$(echo "$remote_raw" | grep '"pid":' | head -n 1 | awk -F':' '{print $2}' | tr -d ' \t\r\n,')
            [ -n "$r_pid" ] && node_pid="$r_pid"

            r_tot=$(echo "$remote_raw" | grep '"total_stations":' | head -n 1 | awk -F':' '{print $2}' | tr -d ' \t\r\n,')
            [ -n "$r_tot" ] && node_total="$r_tot"

            r_2g=$(echo "$remote_raw" | grep '"stations_2g":' | head -n 1 | awk -F':' '{print $2}' | tr -d ' \t\r\n,')
            [ -n "$r_2g" ] && node_2g="$r_2g"

            r_5g=$(echo "$remote_raw" | grep '"stations_5g":' | head -n 1 | awk -F':' '{print $2}' | tr -d ' \t\r\n,')
            [ -n "$r_5g" ] && node_5g="$r_5g"

            r_cu_2g=$(echo "$remote_raw" | grep '"cu_total_2g":' | head -n 1 | awk -F':' '{print $2}' | tr -d ' \t\r\n,')
            [ -n "$r_cu_2g" ] && node_cu_2g="$r_cu_2g"

            r_cu_5g=$(echo "$remote_raw" | grep '"cu_total_5g":' | head -n 1 | awk -F':' '{print $2}' | tr -d ' \t\r\n,')
            [ -n "$r_cu_5g" ] && node_cu_5g="$r_cu_5g"

            r_mode=$(echo "$remote_raw" | grep '"mode":' | head -n 1 | awk -F'"' '{print $4}')
            [ -n "$r_mode" ] && node_mode="$r_mode"

            r_hb=$(echo "$remote_raw" | grep '"last_heartbeat":' | head -n 1 | awk -F'"' '{print $4}')
            [ -n "$r_hb" ] && node_hb="$r_hb"

            r_ver=$(echo "$remote_raw" | sed -n '/---VER---/,/---TEMP---/p' | grep -v -e '---' | tr -d ' \t\r\n')
            r_ver_js=$(echo "$remote_raw" | grep '"version":' | head -n 1 | awk -F'"' '{print $4}' | tr -d ' \t\r\n')
            if [ -n "$r_ver_js" ]; then
                node_version="$r_ver_js"
            elif [ -n "$r_ver" ]; then
                node_version="$r_ver"
            else
                node_version="unknown"
            fi

            if [ "$sync_status" = "Version Match (Config Unverified)" ] && [ "$node_version" != "$main_version" ]; then
                sync_status="Version Diff ($node_version)"
            fi

            r_temp_raw=$(echo "$remote_raw" | sed -n '/---TEMP---/,$p' | grep -oE '^[0-9]+$')
            if [ -n "$r_temp_raw" ] && [ "$r_temp_raw" -gt 0 ]; then
                node_temp=$(awk "BEGIN {printf \"%.1f °C\", $r_temp_raw / 1000}")
            fi
        else
            sync_status="Unreachable"
        fi
    fi

    nodes_json="${nodes_json},
    {
      \"name\": \"${node_name}\",
      \"ip\": \"${node_ip}\",
      \"role\": \"Satellite Node\",
      \"model\": \"${node_model}\",
      \"version\": \"${node_version}\",
      \"status\": \"$( [ "$node_running" = "true" ] && echo "RUNNING" || echo "STOPPED" )\",
      \"pid\": \"${node_pid}\",
      \"total_clients\": ${node_total:-0},
      \"clients_2g\": ${node_2g:-0},
      \"clients_5g\": ${node_5g:-0},
      \"cu_2g\": ${node_cu_2g:-0},
      \"cu_5g\": ${node_cu_5g:-0},
      \"mode\": \"${node_mode}\",
      \"temp\": \"${node_temp}\",
      \"last_heartbeat\": \"${node_hb}\",
      \"sync_status\": \"${sync_status}\"
    }"
done

nodes_json="${nodes_json}
  ]"

# Write cluster.js for WebUI consumption
cat << EOF > "$CLUSTER_JS"
window.unifiBridgeCluster = {
  "last_sync": "${NOW}",
  "nodes": ${nodes_json}
};
EOF

chmod 644 "$CLUSTER_JS"
echo "Cluster sync completed at ${NOW}"
