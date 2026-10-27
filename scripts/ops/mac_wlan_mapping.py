#!/usr/bin/env python3
"""On demand: resolve an explicit UniFi WLAN allowlist on CT107 and publish it."""

from __future__ import annotations

import argparse
import json
import re
import shlex
import subprocess
import sys
from pathlib import Path

import wlan_mapping
import wlan_mapping_publish

DEFAULT_PVE = "root@10.100.1.100"
DEFAULT_CONTROLLER = "https://10.100.1.1"
DEFAULT_SITE = "default"
DEFAULT_AP_HOSTS = ["nithin@10.100.1.10", "nithin@10.100.1.11"]
CT_CREDENTIAL = "/etc/ops-broker/unifi-api-key"
DEFAULT_SSIDS = ["blueberry", "blueberry-prime"]
PVE_HOST_RE = re.compile(r"(?:[A-Za-z0-9_.-]+@)?[A-Za-z0-9][A-Za-z0-9_.:-]{0,252}\Z")


def remote_command(controller: str, site: str, ca_file: str | None, insecure: bool, ssids: list[str] | None = None) -> str:
    origin = wlan_mapping.base_url(controller)
    requested = wlan_mapping.normalize_ssids(ssids if ssids is not None else DEFAULT_SSIDS)
    if not wlan_mapping.SITE_RE.fullmatch(site):
        raise ValueError("site contains unsupported characters")
    if ca_file and insecure:
        raise ValueError("choose either a controller CA file or --insecure")
    args = [
        "/usr/sbin/pct", "exec", "107", "--", "/usr/bin/python3", "-",
        "--controller", origin,
        "--credential-file", CT_CREDENTIAL,
        "--site", site,
    ]
    for ssid in requested:
        args.extend(["--ssid", ssid])
    if ca_file:
        args.extend(["--ca-file", ca_file])
    if insecure:
        args.append("--insecure")
    return " ".join(shlex.quote(arg) for arg in args)


def validate_mapping(raw: bytes, ssids: list[str] | None = None) -> dict[str, str]:
    requested = wlan_mapping.normalize_ssids(ssids if ssids is not None else DEFAULT_SSIDS)

    def unique_pairs(pairs: list[tuple[str, object]]) -> dict[str, object]:
        result: dict[str, object] = {}
        for key, value in pairs:
            if key in result:
                raise ValueError("duplicate mapping name")
            result[key] = value
        return result

    try:
        result = json.loads(raw.decode("utf-8"), object_pairs_hook=unique_pairs)
    except (UnicodeDecodeError, json.JSONDecodeError, ValueError) as exc:
        raise RuntimeError("CT107 returned invalid mapping JSON") from exc
    if not isinstance(result, dict) or set(result) != set(requested):
        raise RuntimeError("CT107 returned an unexpected WLAN mapping")
    if any(not wlan_mapping.valid_legacy_id(value) for value in result.values()):
        raise RuntimeError("CT107 returned an invalid WLAN ObjectId")
    if len(set(result.values())) != len(result):
        raise RuntimeError("CT107 returned duplicate WLAN ObjectIds")
    return {ssid: result[ssid] for ssid in requested}


def resolve_on_ct107(
    pve_host: str,
    controller: str,
    site: str,
    ca_file: str | None,
    insecure: bool,
    ssh: str = "ssh",
    ssids: list[str] | None = None,
) -> dict[str, str]:
    if not PVE_HOST_RE.fullmatch(pve_host):
        raise ValueError("invalid PVE SSH host")
    requested = wlan_mapping.normalize_ssids(ssids if ssids is not None else DEFAULT_SSIDS)
    source = Path(__file__).with_name("wlan_mapping.py").read_bytes()
    cmd = remote_command(controller, site, ca_file, insecure, requested)
    try:
        result = subprocess.run(
            [ssh, "-T", "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=yes", "-o", "ConnectTimeout=8", pve_host, cmd],
            input=source,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            timeout=30,
            check=False,
        )
    except subprocess.TimeoutExpired as exc:
        raise RuntimeError("CT107 WLAN lookup timed out") from exc
    if result.returncode:
        raise RuntimeError(f"CT107 WLAN lookup failed (ssh exit {result.returncode})")
    return validate_mapping(result.stdout, requested)


def refresh(
    pve_host: str,
    controller: str,
    site: str,
    ca_file: str | None,
    insecure: bool,
    ap_hosts: list[str],
    ssh: str = "ssh",
    ssids: list[str] | None = None,
) -> None:
    requested = wlan_mapping.normalize_ssids(ssids if ssids is not None else DEFAULT_SSIDS)
    mapping = resolve_on_ct107(pve_host, controller, site, ca_file, insecure, ssh, requested)
    payload = (json.dumps(mapping, sort_keys=True, separators=(",", ":")) + "\n").encode("utf-8")
    for host in ap_hosts:
        # Keep the Mac's already-authorized SSH identity; CT107 credentials stay local to CT107.
        wlan_mapping_publish.publish(host, payload, ssh=ssh)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--pve-host", default=DEFAULT_PVE)
    parser.add_argument("--controller", default=DEFAULT_CONTROLLER, help="UniFi HTTPS origin")
    parser.add_argument("--site", default=DEFAULT_SITE)
    parser.add_argument("--ca-file", help="CA bundle path on CT107 for controller TLS")
    parser.add_argument("--insecure", action="store_true", help="Disable UniFi certificate verification explicitly")
    parser.add_argument("--ap-host", action="append", dest="ap_hosts", help="AP SSH target; may be repeated")
    parser.add_argument("--ssid", action="append", dest="ssids", help="Exact SSID to include; repeat for each name (default: blueberry and blueberry-prime; maximum 32 unique names)")
    parser.add_argument("--ssh", default="ssh", help="SSH executable using the Mac's existing identity and known_hosts")
    args = parser.parse_args()
    try:
        refresh(args.pve_host, args.controller, args.site, args.ca_file, args.insecure, args.ap_hosts or DEFAULT_AP_HOSTS, args.ssh, args.ssids)
    except (RuntimeError, ValueError, OSError) as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        return 1
    print("WLAN mapping published to configured APs")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
