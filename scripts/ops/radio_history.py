#!/usr/bin/env python3
"""Read-only AirView radio-history check through the existing ops broker."""
import argparse
import json
import math
import shlex
import socket
import ssl
import subprocess
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

PVE = "root@10.100.1.100"
CTID = "107"
BASE = "https://10.100.1.1"
KEY_FILE = "/etc/ops-broker/unifi-api-key"
MAX_BODY = 4 * 1024 * 1024
AP_MACS = {"fc:34:97:46:7b:10": "main", "fc:34:97:4f:ac:60": "node"}
HISTORY_FIELDS = ("bytes", "client_signal_avg", "tx_retries_percentage", "interference_avg")
RETRY_FIELDS = ("tx_success", "tx_total", "tx_retries", "wifi_tx_attempts", "wifi_tx_dropped")


def numeric(value):
    return isinstance(value, (int, float)) and not isinstance(value, bool) and math.isfinite(value)


def project(devices_payload, history_payload):
    """Reduce controller responses to explicitly allowlisted aggregate fields."""
    raw_devices = devices_payload.get("data", []) if isinstance(devices_payload, dict) else []
    raw_history = history_payload.get("radio_details", []) if isinstance(history_payload, dict) else []
    if not isinstance(raw_devices, list):
        raw_devices = []
    device_map = {d["mac"].lower(): d for d in raw_devices
                  if isinstance(d, dict) and isinstance(d.get("mac"), str)}
    history_map = {}
    if isinstance(raw_history, list):
        for row in raw_history:
            if isinstance(row, dict) and isinstance(row.get("device_mac"), str) and isinstance(row.get("radio_name"), str):
                history_map[(row["device_mac"].lower(), row["radio_name"])] = row
    result = {}
    for mac, alias in AP_MACS.items():
        device = device_map.get(mac, {})
        stats = device.get("radio_table_stats")
        stats = stats if isinstance(stats, list) else []
        vaps = device.get("vap_table")
        vaps = vaps if isinstance(vaps, list) else []
        radios = device.get("radio_table")
        radios = radios if isinstance(radios, list) else []
        rows = []
        for radio in radios:
            if not isinstance(radio, dict) or not isinstance(radio.get("name"), str):
                continue
            name = radio["name"]
            stat = next((x for x in stats if isinstance(x, dict) and x.get("name") == name), {})
            active = [v for v in vaps if isinstance(v, dict) and v.get("radio_name") == name and numeric(v.get("num_sta")) and v["num_sta"] > 0]
            hrow = history_map.get((mac, name), {})
            history = {k: hrow.get(k) if numeric(hrow.get(k)) else None for k in HISTORY_FIELDS}
            item = {
                "name": name,
                "num_sta": stat.get("num_sta") if numeric(stat.get("num_sta")) and stat["num_sta"] >= 0 else None,
                "bw": stat.get("bw") if numeric(stat.get("bw")) and stat["bw"] > 0 else None,
                "cu_total": stat.get("cu_total") if numeric(stat.get("cu_total")) and 0 <= stat["cu_total"] <= 100 else None,
                "live_vaps": {"active": len(active),
                              "signal_measured": sum(numeric(v.get("avg_client_signal")) and -127 <= v["avg_client_signal"] < 0 for v in active),
                              "retry_counters_measured": sum(all(numeric(v.get(k)) and v[k] >= 0 for k in RETRY_FIELDS) for v in active)},
                "history": history,
            }
            values = list(history.values())
            if (item["num_sta"] or 0) > 0 and values and all(v is not None and v == 0 for v in values):
                item["observation"] = "history_summary_zero_with_active_clients"
            rows.append(item)
        result[alias] = rows
    return result


def fetch_json(path, context, key):
    request = urllib.request.Request(BASE + path, headers={"X-API-Key": key, "Accept": "application/json"})
    try:
        with urllib.request.urlopen(request, context=context, timeout=20) as response:
            body = response.read(MAX_BODY + 1)
    except urllib.error.HTTPError as exc:
        raise RuntimeError("controller HTTP status " + str(exc.code)) from None
    except Exception:
        raise RuntimeError("controller transport or TLS failure") from None
    if len(body) > MAX_BODY:
        raise RuntimeError("controller response exceeds 4 MiB limit")
    try:
        return json.loads(body)
    except Exception:
        raise RuntimeError("controller returned invalid JSON") from None


def tls_context(insecure=False, ca_file=None):
    return ssl._create_unverified_context() if insecure else ssl.create_default_context(cafile=ca_file)


def remote(args):
    try:
        if socket.gethostname() != "core-ops-broker":
            raise RuntimeError("unexpected CT107 execution context")
        with open(KEY_FILE, encoding="utf-8") as handle:
            key = handle.read().strip()
        if not key:
            raise RuntimeError("credential unavailable")
        context = tls_context(args.insecure, args.ca_file)
        end = int(time.time() * 1000)
        start = end - 86_400_000
        devices = fetch_json("/proxy/network/api/s/default/stat/device", context, key)
        query = urllib.parse.urlencode({"start": start, "end": end})
        history = fetch_json("/proxy/network/v2/api/site/default/wifi-stats/radios?" + query, context, key)
        print(json.dumps(project(devices, history), separators=(",", ":")))
        return 0
    except Exception as exc:
        message = str(exc) if isinstance(exc, RuntimeError) else "remote diagnostic failed"
        print(json.dumps({"error": message[:100]}))
        return 2


def cli():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--remote", action="store_true", help=argparse.SUPPRESS)
    tls = parser.add_mutually_exclusive_group()
    tls.add_argument("--insecure", action="store_true", help="disable controller TLS validation")
    tls.add_argument("--ca-file", help="CA file path on CT107")
    args = parser.parse_args()
    if args.remote:
        return remote(args)
    remote_args = ["/usr/sbin/pct", "exec", CTID, "--", "/usr/bin/python3", "-", "--remote"]
    if args.insecure:
        remote_args.append("--insecure")
    elif args.ca_file:
        remote_args.extend(["--ca-file", args.ca_file])
    command = ["ssh", "-T", "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=yes",
               "-o", "ConnectTimeout=10", PVE, shlex.join(remote_args)]
    try:
        with open(__file__, "r", encoding="utf-8") as source:
            process = subprocess.run(command, input=source.read(), text=True, capture_output=True, timeout=50, check=False)
    except subprocess.TimeoutExpired:
        print("diagnostic timed out", file=sys.stderr)
        return 2
    except OSError:
        print("strict-host-verified SSH could not run", file=sys.stderr)
        return 2
    if process.returncode != 0:
        print("remote diagnostic failed; details suppressed", file=sys.stderr)
        return 2
    try:
        result = json.loads(process.stdout)
    except Exception:
        print("remote diagnostic returned invalid output", file=sys.stderr)
        return 2
    if "error" in result:
        print("diagnostic error: " + str(result["error"]), file=sys.stderr)
        return 2
    print(json.dumps(result, indent=2, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(cli())
