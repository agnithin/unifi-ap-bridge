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
main_model="UniFi 6 Mesh (U6M)"
main_mode="GCM"
main_heartbeat="-"
main_temp="-"

if [ -f "$main_status_js" ]; then
    main_total=$(grep '"total_stations":' "$main_status_js" | head -n 1 | awk -F':' '{print $2}' | tr -d ' \t\r\n,')
    main_2g=$(grep '"stations_2g":' "$main_status_js" | head -n 1 | awk -F':' '{print $2}' | tr -d ' \t\r\n,')
    main_5g=$(grep '"stations_5g":' "$main_status_js" | head -n 1 | awk -F':' '{print $2}' | tr -d ' \t\r\n,')
    m_val=$(grep '"model":' "$main_status_js" | head -n 1 | awk -F'"' '{print $4}')
    [ -n "$m_val" ] && main_model="$m_val"
    m_mode=$(grep '"mode":' "$main_status_js" | head -n 1 | awk -F'"' '{print $4}')
    [ -n "$m_mode" ] && main_mode="$m_mode"
    m_hb=$(grep '"last_heartbeat":' "$main_status_js" | head -n 1 | awk -F'"' '{print $4}')
    [ -n "$m_hb" ] && main_heartbeat="$m_hb"
fi

main_version=$(cat "$INSTALL_DIR/version.txt" 2>/dev/null || echo "1.0.0")

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
      \"mode\": \"${main_mode}\",
      \"temp\": \"${main_temp}\",
      \"last_heartbeat\": \"${main_heartbeat}\",
      \"sync_status\": \"In Sync (Local)\"
    }"

# 2. Iterate Satellite Nodes
NODE_IPS=$(get_node_ips)

for node_ip in $NODE_IPS; do
    [ -z "$node_ip" ] && continue
    sync_status="In Sync"
    node_running="false"
    node_name="ZenWiFi_Node_${node_ip##*.}"
    node_model="$main_model"
    node_version="$main_version"
    node_pid="-"
    node_total=0
    node_2g=0
    node_5g=0
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
            tar -czf - -C "$INSTALL_DIR" apply_config.sh sync_nodes.sh update.sh watchdog.sh version.txt webui config/config.json unifi-bridge.tar.gz 2>/dev/null | \
                ssh -i "$SSH_KEY" -y "$node_ip" "tar -xzf - -C $INSTALL_DIR 2>/dev/null; [ -f $INSTALL_DIR/update.sh ] && chmod 755 $INSTALL_DIR/*.sh; $INSTALL_DIR/start.sh" >/dev/null 2>&1
            if [ $? -eq 0 ]; then
                sync_status="Updated & In Sync"
            else
                sync_status="Update Failed"
            fi
        # If action is 'sync', push local config to remote node
        elif [ "$ACTION" = "sync" ] && [ -f "$CONFIG_FILE" ]; then
            # Remote config with node-specific state path
            ssh -i "$SSH_KEY" -y "$node_ip" "mkdir -p $INSTALL_DIR/config; cat > $INSTALL_DIR/config/config.json && $INSTALL_DIR/start.sh" < "$CONFIG_FILE" >/dev/null 2>&1
            if [ $? -eq 0 ]; then
                sync_status="In Sync"
            else
                sync_status="Sync Failed"
            fi
        fi

        # Query remote node status
        remote_raw=$(ssh -i "$SSH_KEY" -y "$node_ip" "cat /tmp/var/wwwext/$ADDON_NAME/status.js 2>/dev/null; echo '---VER---'; cat $INSTALL_DIR/version.txt 2>/dev/null; echo '---TEMP---'; cat /sys/class/thermal/thermal_zone0/temp 2>/dev/null" </dev/null 2>/dev/null)
        if [ -n "$remote_raw" ]; then
            r_host=$(echo "$remote_raw" | grep '"hostname":' | head -n 1 | awk -F'"' '{print $4}')
            [ -n "$r_host" ] && node_name="$r_host"

            r_run=$(echo "$remote_raw" | grep '"running":' | head -n 1 | awk -F':' '{print $2}' | tr -d ' \t\r\n,')
            [ "$r_run" = "true" ] && node_running="true"

            r_pid=$(echo "$remote_raw" | grep '"pid":' | head -n 1 | awk -F':' '{print $2}' | tr -d ' \t\r\n,')
            [ -n "$r_pid" ] && node_pid="$r_pid"

            r_tot=$(echo "$remote_raw" | grep '"total_stations":' | head -n 1 | awk -F':' '{print $2}' | tr -d ' \t\r\n,')
            [ -n "$r_tot" ] && node_total="$r_tot"

            r_2g=$(echo "$remote_raw" | grep '"stations_2g":' | head -n 1 | awk -F':' '{print $2}' | tr -d ' \t\r\n,')
            [ -n "$r_2g" ] && node_2g="$r_2g"

            r_5g=$(echo "$remote_raw" | grep '"stations_5g":' | head -n 1 | awk -F':' '{print $2}' | tr -d ' \t\r\n,')
            [ -n "$r_5g" ] && node_5g="$r_5g"

            r_mode=$(echo "$remote_raw" | grep '"mode":' | head -n 1 | awk -F'"' '{print $4}')
            [ -n "$r_mode" ] && node_mode="$r_mode"

            r_hb=$(echo "$remote_raw" | grep '"last_heartbeat":' | head -n 1 | awk -F'"' '{print $4}')
            [ -n "$r_hb" ] && node_hb="$r_hb"

            r_ver=$(echo "$remote_raw" | sed -n '/---VER---/,/---TEMP---/p' | grep -vE '---' | tr -d ' \t\r\n')
            if [ -n "$r_ver" ]; then
                node_version="$r_ver"
            else
                r_ver_js=$(echo "$remote_raw" | grep '"version":' | head -n 1 | awk -F'"' '{print $4}' | tr -d ' \t\r\n')
                [ -n "$r_ver_js" ] && node_version="$r_ver_js"
            fi

            if [ "$sync_status" = "In Sync" ] && [ "$node_version" != "$main_version" ]; then
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
