#!/usr/bin/env python3
"""Refresh the allowlisted UniFi WLAN mapping and publish it to AP RAM files.

Run this on the trusted host that holds the UniFi API key and AP SSH identity.
The only remote write is a fixed atomic rename to the bridge's private JFFS
mapping path, skipped when content is unchanged. No controller objects or
credentials are sent to an AP.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import subprocess
import sys
from pathlib import Path
from typing import Any

import wlan_mapping

DEFAULT_CONFIG = Path("/etc/ops-broker/unifi-wlan-mapping.json")
DEFAULT_CREDENTIAL = Path("/etc/ops-broker/unifi-api-key")
SSIDS = ["blueberry", "blueberry-prime"]
REMOTE_COMMAND = (
    "umask 077; mkdir -p /jffs/addons/unifi-bridge/config && "
    "tmp=/jffs/addons/unifi-bridge/config/wlan-mapping.json.tmp.$$ && "
    "trap 'rm -f \"$tmp\"' EXIT; cat > \"$tmp\" && chmod 600 \"$tmp\" && "
    "mv -f \"$tmp\" /jffs/addons/unifi-bridge/config/wlan-mapping.json"
)
REMOTE_CHECKSUM_COMMAND = (
    "if [ -f /jffs/addons/unifi-bridge/config/wlan-mapping.json ]; then "
    "mode=$(stat -c '%a' /jffs/addons/unifi-bridge/config/wlan-mapping.json) && "
    "digest=$(sha256sum /jffs/addons/unifi-bridge/config/wlan-mapping.json) && "
    "printf '%s %s\\n' \"$mode\" \"${digest%% *}\"; else printf 'missing\\n'; fi"
)
HOST_RE = re.compile(r"(?:[A-Za-z0-9_.-]+@)?[A-Za-z0-9][A-Za-z0-9_.:-]{0,252}\Z")


def load_config(path: Path) -> dict[str, Any]:
    try:
        value = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        raise ValueError("mapping deployment config is unreadable or invalid") from exc
    if not isinstance(value, dict):
        raise ValueError("mapping deployment config must be an object")
    controller = value.get("controller")
    hosts = value.get("ap_hosts")
    if not isinstance(controller, str) or not isinstance(hosts, list) or not hosts:
        raise ValueError("config requires controller and a non-empty ap_hosts list")
    if any(not isinstance(host, str) or not HOST_RE.fullmatch(host) for host in hosts):
        raise ValueError("AP host entries must be simple SSH host names or user@host")
    if len(set(hosts)) != len(hosts):
        raise ValueError("AP host entries must be unique")
    site = value.get("site", "default")
    if not isinstance(site, str) or not wlan_mapping.SITE_RE.fullmatch(site):
        raise ValueError("site contains unsupported characters")
    identity_file = value.get("identity_file")
    if not isinstance(identity_file, str) or not identity_file:
        raise ValueError("config requires the dedicated CT107 SSH identity_file path")
    identity = Path(identity_file)
    try:
        identity_info = identity.stat()
    except OSError as exc:
        raise ValueError("dedicated SSH identity file is unavailable") from exc
    if not identity.is_file() or identity_info.st_mode & 0o077:
        raise ValueError("dedicated SSH identity file must be regular and private")
    ca_file = value.get("ca_file")
    insecure = value.get("insecure", False)
    if ca_file is not None and (not isinstance(ca_file, str) or not ca_file):
        raise ValueError("ca_file must be a non-empty path")
    if not isinstance(insecure, bool) or (ca_file is not None and insecure):
        raise ValueError("choose either ca_file or explicit insecure TLS mode")
    return {
        "controller": controller,
        "site": site,
        "ap_hosts": hosts,
        "identity_file": identity_file,
        "ca_file": ca_file,
        "insecure": insecure,
    }


def remote_state(ssh_args: list[str], host: str) -> tuple[str, str] | None:
    try:
        result = subprocess.run(
            [*ssh_args, REMOTE_CHECKSUM_COMMAND],
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            timeout=15,
            check=False,
        )
    except subprocess.TimeoutExpired as exc:
        raise RuntimeError(f"mapping check timed out for {host}") from exc
    if result.returncode:
        raise RuntimeError(f"mapping check failed for {host} (ssh exit {result.returncode})")
    try:
        fields = result.stdout.decode("ascii", errors="strict").split()
    except UnicodeDecodeError as exc:
        raise RuntimeError(f"mapping check returned invalid state for {host}") from exc
    if fields == ["missing"]:
        return None
    if len(fields) != 2 or not re.fullmatch(r"[0-9a-fA-F]{64}", fields[1]):
        raise RuntimeError(f"mapping check returned invalid state for {host}")
    return fields[0], fields[1].lower()


def publish(host: str, payload: bytes, identity_file: str, ssh: str = "ssh") -> None:
    if not HOST_RE.fullmatch(host):
        raise ValueError("invalid AP SSH host")
    ssh_args = [
        ssh, "-T", "-i", identity_file, "-o", "IdentitiesOnly=yes", "-o", "BatchMode=yes",
        "-o", "StrictHostKeyChecking=yes", "-o", "ConnectTimeout=5", host,
    ]
    expected_digest = hashlib.sha256(payload).hexdigest()
    current = remote_state(ssh_args, host)
    if current == ("600", expected_digest):
        return
    try:
        result = subprocess.run(
            [*ssh_args, REMOTE_COMMAND],
            input=payload,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.PIPE,
            timeout=15,
            check=False,
        )
    except subprocess.TimeoutExpired as exc:
        raise RuntimeError(f"mapping publish timed out for {host}") from exc
    if result.returncode:
        raise RuntimeError(f"mapping publish failed for {host} (ssh exit {result.returncode})")
    if remote_state(ssh_args, host) != ("600", expected_digest):
        raise RuntimeError(f"mapping verification failed for {host}")


def refresh(config_path: Path, credential_path: Path, ssh: str = "ssh") -> None:
    config = load_config(config_path)
    api_key = wlan_mapping.credential(credential_path)
    context = wlan_mapping.tls_context(Path(config["ca_file"]) if config["ca_file"] else None, config["insecure"])
    mapping = wlan_mapping.fetch_mapping(config["controller"], config["site"], api_key, SSIDS, context)
    # Re-serialize only resolved values; no raw controller record reaches SSH.
    payload = (json.dumps(mapping, sort_keys=True, separators=(",", ":")) + "\n").encode("utf-8")
    for host in config["ap_hosts"]:
        publish(host, payload, config["identity_file"], ssh)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--config", type=Path, default=DEFAULT_CONFIG)
    parser.add_argument("--credential-file", type=Path, default=DEFAULT_CREDENTIAL)
    parser.add_argument("--ssh", default="ssh", help="SSH executable; normally resolved from the host PATH")
    args = parser.parse_args()
    try:
        refresh(args.config, args.credential_file, args.ssh)
    except (RuntimeError, ValueError) as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        return 1
    print("WLAN mapping refreshed on configured APs")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
