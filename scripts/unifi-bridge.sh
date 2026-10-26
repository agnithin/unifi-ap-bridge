#!/bin/sh
# ==============================================================================
# UniFi AP Bridge - CLI Management Tool & amtm Companion
# ==============================================================================

ADDON_NAME="unifi-bridge"
INSTALL_DIR="/jffs/addons/$ADDON_NAME"
CONFIG_FILE="$INSTALL_DIR/config/config.json"
STATE_FILE="$INSTALL_DIR/state.json"
LOG_FILE="/tmp/unifi-bridge.log"
STATUS_JS="/tmp/var/wwwext/unifi-bridge/status.js"
DAEMON_BIN="/tmp/unifi-bridge/unifi-bridge"

# Terminal Colors
C_RESET="\033[0m"
C_BOLD="\033[1m"
C_GREEN="\033[1;32m"
C_RED="\033[1;31m"
C_YELLOW="\033[1;33m"
C_CYAN="\033[1;36m"
C_BLUE="\033[1;34m"

get_pid() {
    pidof unifi-bridge 2>/dev/null | awk '{print $1}'
}

is_running() {
    pid=$(get_pid)
    [ -n "$pid" ]
}

show_header() {
    printf "${C_CYAN}${C_BOLD}"
    echo "====================================================="
    echo "       UniFi AP Bridge Manager for Asuswrt-Merlin    "
    echo "====================================================="
    printf "${C_RESET}"
}

cmd_status() {
    show_header
    pid=$(get_pid)
    if [ -n "$pid" ]; then
        printf "Service Status   : ${C_GREEN}RUNNING${C_RESET} (PID: %s)\n" "$pid"
    else
        printf "Service Status   : ${C_RED}STOPPED${C_RESET}\n"
    fi

    ver="1.0.0"
    [ -f "$INSTALL_DIR/version.txt" ] && ver=$(cat "$INSTALL_DIR/version.txt" 2>/dev/null | tr -d ' \t\r\n')
    branch="main"
    [ -f "$CONFIG_FILE" ] && branch=$(grep '"branch"' "$CONFIG_FILE" 2>/dev/null | head -n 1 | awk -F'"' '{print $4}')
    [ -z "$branch" ] && branch="main"
    printf "Software Version : ${C_BOLD}%s${C_RESET} [Track: %s]\n" "$ver" "$branch"

    # Read config
    if [ -f "$CONFIG_FILE" ]; then
        ctrl_url=$(grep '"controller_url"' "$CONFIG_FILE" | head -n 1 | awk -F'"' '{print $4}')
        model=$(grep '"model"' "$CONFIG_FILE" | head -n 1 | awk -F'"' '{print $4}')
        interval=$(grep '"interval_sec"' "$CONFIG_FILE" | head -n 1 | awk -F':' '{print $2}' | tr -d ', ')
        printf "Emulated AP Model: %s\n" "${model:-U6M}"
        printf "Inform Endpoint  : %s\n" "${ctrl_url:-Unknown}"
        printf "Report Interval  : %s seconds\n" "${interval:-10}"
    fi

    # WebUI Slot
    slot_file="$INSTALL_DIR/.user_slot"
    if [ -f "$slot_file" ]; then
        slot=$(cat "$slot_file" 2>/dev/null | tr -d ' \t\r\n')
        ip=$(nvram get lan_ipaddr)
        printf "WebUI Location   : http://%s/%s.asp (Wireless tab)\n" "$ip" "${slot:-user3}"
    fi

    # Live Associated Stations
    total_sta=0
    sta_2g=0
    sta_5g=0
    load_2g=""
    load_5g=""
    if [ -f "$STATUS_JS" ]; then
        total_sta=$(grep '"total_stations":' "$STATUS_JS" | head -n 1 | awk -F':' '{print $2}' | tr -d ' \t\r\n,')
        sta_2g=$(grep '"stations_2g":' "$STATUS_JS" | head -n 1 | awk -F':' '{print $2}' | tr -d ' \t\r\n,')
        sta_5g=$(grep '"stations_5g":' "$STATUS_JS" | head -n 1 | awk -F':' '{print $2}' | tr -d ' \t\r\n,')
        load_2g=$(grep '"cu_total_2g":' "$STATUS_JS" | head -n 1 | awk -F':' '{print $2}' | tr -d ' \t\r\n,')
        load_5g=$(grep '"cu_total_5g":' "$STATUS_JS" | head -n 1 | awk -F':' '{print $2}' | tr -d ' \t\r\n,')
    fi

    if [ -z "$total_sta" ] || [ "$total_sta" -eq 0 ]; then
        for iface in eth4 eth5 eth6 wl0.1 wl0.2 wl0.3 wl1.1 wl1.2 wl1.3 wl2.1 wl2.2 wl2.3; do
            count=$(wl -i "$iface" assoclist 2>/dev/null | grep -c 'assoclist' || true)
            if [ "$count" -gt 0 ]; then
                total_sta=$((total_sta + count))
                case "$iface" in
                    eth4|wl0*) sta_2g=$((sta_2g + count)) ;;
                    *)         sta_5g=$((sta_5g + count)) ;;
                esac
            fi
        done
    fi

    echo "-----------------------------------------------------"
    printf "Live Associated Stations & Channel Utilization:\n"
    if [ -n "$load_2g" ] && [ "$load_2g" -gt 0 ] 2>/dev/null; then
        printf "  - 2.4 GHz Band (IoT/Prime) : %s clients [Channel Load: %s%%]\n" "${sta_2g:-0}" "$load_2g"
    else
        printf "  - 2.4 GHz Band (IoT/Prime) : %s clients\n" "${sta_2g:-0}"
    fi
    if [ -n "$load_5g" ] && [ "$load_5g" -gt 0 ] 2>/dev/null; then
        printf "  - 5 GHz High-Speed Band    : %s clients [Channel Load: %s%%]\n" "${sta_5g:-0}" "$load_5g"
    else
        printf "  - 5 GHz High-Speed Band    : %s clients\n" "${sta_5g:-0}"
    fi
    printf "  ${C_BOLD}Total Wireless Stations    : %s clients${C_RESET}\n" "${total_sta:-0}"

    # CPU & Memory Resources
    if [ -f "/proc/loadavg" ]; then
        load_1m=$(awk '{print $1}' /proc/loadavg)
        printf "System CPU Load  : %s (1m avg)\n" "$load_1m"
    fi
    if [ -f "/proc/meminfo" ]; then
        mem_tot=$(grep MemTotal /proc/meminfo | awk '{print $2}')
        mem_free=$(grep MemFree /proc/meminfo | awk '{print $2}')
        mem_buf=$(grep Buffers /proc/meminfo | awk '{print $2}')
        mem_cached=$(grep '^Cached:' /proc/meminfo | awk '{print $2}')
        if [ -n "$mem_tot" ] && [ "$mem_tot" -gt 0 ]; then
            mem_used=$((mem_tot - mem_free - mem_buf - mem_cached))
            mem_pct=$(( (mem_used * 100) / mem_tot ))
            printf "System Memory    : %s%% used (%s MB / %s MB)\n" "$mem_pct" "$((mem_used / 1024))" "$((mem_tot / 1024))"
        fi
    fi

    # CPU Temperature if available
    if [ -f "/sys/class/thermal/thermal_zone0/temp" ]; then
        raw_temp=$(cat /sys/class/thermal/thermal_zone0/temp 2>/dev/null)
        if [ -n "$raw_temp" ] && [ "$raw_temp" -gt 0 ]; then
            temp_c=$(awk "BEGIN {printf \"%.1f\", $raw_temp / 1000}")
            printf "Hardware CPU Temp: %s °C\n" "$temp_c"
        fi
    fi

    # Physical Ethernet Switch Ports
    echo "-----------------------------------------------------"
    printf "Physical Ethernet Switch Ports:\n"
    for p in 0 1 2 3; do
        iface="eth$p"
        pname="Port $((p + 1))"
        case "$p" in
            0) pname="Port 1 (2.5G Uplink)" ;;
            1) pname="Port 2 (LAN 1)" ;;
            2) pname="Port 3 (LAN 2)" ;;
            3) pname="Port 4 (LAN 3)" ;;
        esac
        if [ -d "/sys/class/net/$iface" ]; then
            c=$(cat "/sys/class/net/$iface/carrier" 2>/dev/null || echo 0)
            if [ "$c" = "1" ]; then
                sp=$(cat "/sys/class/net/$iface/speed" 2>/dev/null || echo "Unknown")
                dup=$(cat "/sys/class/net/$iface/duplex" 2>/dev/null || echo "")
                printf "  - %-22s : ${C_GREEN}UP${C_RESET} at %s Mbps %s\n" "$pname" "$sp" "$dup"
            else
                printf "  - %-22s : ${C_RED}DOWN${C_RESET}\n" "$pname"
            fi
        fi
    done

    # Cluster Nodes Status if available
    CLUSTER_JS="/tmp/var/wwwext/$ADDON_NAME/cluster.js"
    if [ -f "$CLUSTER_JS" ]; then
        echo "-----------------------------------------------------"
        printf "${C_BOLD}Cluster Access Points & Synchronization:${C_RESET}\n"
        awk -F'"' '
            /"name":/ { name=$4 }
            /"ip":/ { ip=$4 }
            /"role":/ { role=$4 }
            /"status":/ { st=$4 }
            /"total_clients":/ { gsub(/[^0-9]/, "", $0); tot=$0 }
            /"sync_status":/ {
                sync=$4
                printf "  - %-18s (%s) [%s]: %s | %s clients | %s\n", name, ip, role, st, tot, sync
            }
        ' "$CLUSTER_JS"
        sync_time=$(grep '"last_sync":' "$CLUSTER_JS" | head -n 1 | awk -F'"' '{print $4}')
        printf "Last Sync: %s\n" "${sync_time:--}"
    fi

    # Last log event
    if [ -f "$LOG_FILE" ]; then
        echo "-----------------------------------------------------"
        printf "Recent Heartbeat:\n"
        tail -n 2 "$LOG_FILE" | sed 's/^/  /'
    fi
    echo "====================================================="
}

cmd_sync() {
    show_header
    printf "${C_CYAN}Synchronizing configuration to Satellite Nodes...${C_RESET}\n"
    if [ -f "$INSTALL_DIR/sync_nodes.sh" ]; then
        "$INSTALL_DIR/sync_nodes.sh" sync
    else
        printf "${C_RED}sync_nodes.sh not found.${C_RESET}\n"
    fi
}

cmd_start() {
    if is_running; then
        printf "${C_YELLOW}UniFi AP Bridge is already running (PID: %s).${C_RESET}\n" "$(get_pid)"
        return 0
    fi
    printf "${C_GREEN}Starting UniFi AP Bridge...${C_RESET}\n"
    /jffs/addons/unifi-bridge/start.sh
    sleep 2
    if is_running; then
        printf "${C_GREEN}Started successfully (PID: %s).${C_RESET}\n" "$(get_pid)"
    else
        printf "${C_RED}Failed to start. Check /tmp/unifi-bridge.log.${C_RESET}\n"
    fi
}

cmd_stop() {
    if ! is_running; then
        printf "${C_YELLOW}UniFi AP Bridge is not running.${C_RESET}\n"
        return 0
    fi
    printf "${C_YELLOW}Stopping UniFi AP Bridge (PID: %s)...${C_RESET}\n" "$(get_pid)"
    killall unifi-bridge 2>/dev/null || true
    sleep 1
    printf "${C_GREEN}Stopped successfully.${C_RESET}\n"
}

cmd_restart() {
    printf "${C_CYAN}Restarting UniFi AP Bridge...${C_RESET}\n"
    cmd_stop
    cmd_start
}

cmd_logs() {
    if [ ! -f "$LOG_FILE" ]; then
        echo "No log file found at $LOG_FILE"
        return 1
    fi
    printf "${C_CYAN}Streaming live logs from %s (Ctrl+C to exit)...${C_RESET}\n" "$LOG_FILE"
    tail -f -n 20 "$LOG_FILE"
}

cmd_test() {
    show_header
    ctrl_url=""
    if [ -f "$CONFIG_FILE" ]; then
        extracted=$(grep '"controller_url"' "$CONFIG_FILE" | head -n 1 | awk -F'"' '{print $4}')
        [ -n "$extracted" ] && ctrl_url="$extracted"
    fi

    if [ -z "$ctrl_url" ]; then
        printf "No controller URL is configured; set an explicit gateway or controller URL first.\n"
        return 1
    fi

    printf "Testing controller reachability: %s\n" "$ctrl_url"
    resp=$(curl -s -o /dev/null -w "%{http_code}" --connect-timeout 3 --max-time 5 "$ctrl_url" || echo "failed")
    if [ "$resp" = "400" ] || [ "$resp" = "200" ] || [ "$resp" = "404" ]; then
        printf "${C_GREEN}✓ Controller reachable! HTTP response code: %s${C_RESET}\n" "$resp"
    else
        printf "${C_RED}✗ Controller unreachable (status / error: %s)${C_RESET}\n" "$resp"
    fi
}

cmd_update() {
    sub="${1:-check}"
    branch="${2:-}"
    if [ -f "$INSTALL_DIR/update.sh" ]; then
        "$INSTALL_DIR/update.sh" "$sub" "$branch"
    else
        printf "${C_RED}update.sh not found in %s${C_RESET}\n" "$INSTALL_DIR"
    fi
}

cmd_menu() {
    while true; do
        clear
        show_header
        pid=$(get_pid)
        if [ -n "$pid" ]; then
            printf " Service State: ${C_GREEN}RUNNING${C_RESET} [PID: %s]\n\n" "$pid"
        else
            printf " Service State: ${C_RED}STOPPED${C_RESET}\n\n"
        fi

        echo " 1) Detailed Service Status"
        echo " 2) Live Heartbeat Log (tail -f)"
        echo " 3) Restart Service"
        echo " 4) Stop Service"
        echo " 5) Start Service"
        echo " 6) Test Controller Reachability"
        echo " 7) Check for Updates / Apply Update"
        echo " 8) Re-mount WebUI Tab"
        echo " e) Exit"
        echo ""
        printf " Enter selection [1-8 or e]: "
        read -r choice
        case "$choice" in
            1)
                echo ""
                cmd_status
                printf "\nPress Enter to return to menu..."; read -r _ ;;
            2)
                echo ""
                cmd_logs ;;
            3)
                echo ""
                cmd_restart
                sleep 2 ;;
            4)
                echo ""
                cmd_stop
                sleep 2 ;;
            5)
                echo ""
                cmd_start
                sleep 2 ;;
            6)
                echo ""
                cmd_test
                printf "\nPress Enter to return to menu..."; read -r _ ;;
            7)
                echo ""
                printf "${C_CYAN}Choose branch track ([m]ain / [d]ev, default main): ${C_RESET}"
                read -r trk
                b="main"
                [ "$trk" = "d" ] || [ "$trk" = "dev" ] && b="dev"
                cmd_update check "$b"
                echo ""
                printf "Do you want to apply the update now? [y/N]: "
                read -r ans
                if [ "$ans" = "y" ] || [ "$ans" = "Y" ]; then
                    cmd_update apply "$b"
                fi
                printf "\nPress Enter to return to menu..."; read -r _ ;;
            8)
                echo ""
                printf "${C_CYAN}Re-mounting WebUI tab...${C_RESET}\n"
                /jffs/addons/unifi-bridge/start.sh
                printf "${C_GREEN}Done!${C_RESET}\n"
                sleep 2 ;;
            e|E|q|Q)
                echo "Exiting."
                break ;;
            *)
                echo "Invalid option."
                sleep 1 ;;
        esac
    done
}

cmd_reset() {
    show_header
    printf "${C_YELLOW}Resetting UniFi AP adoption identity...${C_RESET}\n"
    rm -f "$STATE_FILE"
    killall unifi-bridge 2>/dev/null || true
    sleep 1
    "$INSTALL_DIR/start.sh"
    printf "${C_GREEN}Done! The AP will now broadcast as an unadopted device ready for adoption in the UniFi Console.${C_RESET}\n"
}

case "${1:-}" in
    status)
        cmd_status ;;
    start)
        cmd_start ;;
    stop)
        cmd_stop ;;
    restart)
        cmd_restart ;;
    reset|re-adopt)
        cmd_reset ;;
    logs|log)
        cmd_logs ;;
    test|check)
        cmd_test ;;
    sync)
        cmd_sync ;;
    update)
        cmd_update "${2:-check}" "${3:-}" ;;
    "")
        cmd_menu ;;
    *)
        echo "Usage: $0 {status|sync|update [check|apply] [branch]|start|stop|restart|reset|logs|test}"
        exit 1 ;;
esac
