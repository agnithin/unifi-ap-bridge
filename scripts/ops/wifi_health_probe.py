#!/usr/bin/env python3
"""Check a connected Linux Wi-Fi client from outside the bridge."""
import argparse
import ipaddress
import json
import re
import subprocess
import sys


def run(*args):
    result = subprocess.run(args, text=True, capture_output=True, timeout=5, check=True)
    return result.stdout


def probe(interface, dns_server, dns_name, expected_bssid=None, run_command=run):
    result = dict(association=False, dhcp_ipv4=False, dns=False)
    try:
        link = run_command("iw", "dev", interface, "link")
        match = re.search(r"^Connected to ([0-9a-f:]{17})\b", link, re.M | re.I)
        if not match or (expected_bssid and match[1].lower() != expected_bssid.lower()):
            return result
        result["association"] = True
        # Require NetworkManager DHCP data on this interface and a live address.
        lease = run_command("nmcli", "-g", "DHCP4.OPTION", "device", "show", interface)
        address = re.search(r"\bip_address\s*=\s*([0-9.]+)\b", lease)
        if not address:
            return result
        source = str(ipaddress.IPv4Address(address[1]))
        interfaces = json.loads(run_command("ip", "-j", "-4", "addr", "show", "dev", interface))
        if not any(a["local"] == source for i in interfaces for a in i.get("addr_info", [])):
            return result
        result["dhcp_ipv4"] = True
        routes = json.loads(run_command("ip", "-j", "-4", "route", "get", dns_server, "from", source))
        if not routes or routes[0].get("dev") != interface:
            return result
        answer = run_command("dig", "+time=2", "+tries=1", "+noall", "+answer",
                             "-b", source, "@" + dns_server, dns_name, "A")
        result["dns"] = any(re.search(r"\sIN\s+A\s+\d", line) for line in answer.splitlines())
    except (subprocess.SubprocessError, OSError, ValueError, KeyError, TypeError):
        pass  # Return completed checks without leaking client or resolver details.
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--interface", required=True)
    parser.add_argument("--dns-server", required=True, type=ipaddress.IPv4Address)
    parser.add_argument("--dns-name", required=True, help="Name expected to have an A record")
    parser.add_argument("--bssid", help="Optional AP BSSID, to avoid counting roaming to another AP")
    args = parser.parse_args()
    if not re.fullmatch(r"[A-Za-z0-9_][A-Za-z0-9_.-]{0,14}", args.interface):
        parser.error("invalid interface name")
    if not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9.-]{0,252}", args.dns_name):
        parser.error("invalid DNS name")
    if args.bssid and not re.fullmatch(r"[0-9A-Fa-f]{2}(?::[0-9A-Fa-f]{2}){5}", args.bssid):
        parser.error("invalid BSSID")
    result = probe(args.interface, str(args.dns_server), args.dns_name, args.bssid)
    print(json.dumps(result))
    return 0 if all(result.values()) else 1


if __name__ == "__main__":
    sys.exit(main())
