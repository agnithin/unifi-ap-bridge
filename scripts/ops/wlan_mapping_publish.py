#!/usr/bin/env python3
"""Publish a resolved WLAN mapping to an AP's private persistent cache."""

from __future__ import annotations

import hashlib
import re
import subprocess

AP_CACHE_PATH = "/jffs/addons/unifi-bridge/config/wlan-mapping.json"
REMOTE_COMMAND = (
    "umask 077; mkdir -p /jffs/addons/unifi-bridge/config && "
    "tmp=/jffs/addons/unifi-bridge/config/wlan-mapping.json.tmp.$$ && "
    "trap 'rm -f \"$tmp\"' EXIT; cat > \"$tmp\" && chmod 600 \"$tmp\" && "
    "mv -f \"$tmp\" /jffs/addons/unifi-bridge/config/wlan-mapping.json"
)
REMOTE_CHECKSUM_COMMAND = (
    f"if [ -f {AP_CACHE_PATH} ]; then "
    f"line=$(ls -ld {AP_CACHE_PATH}) && "
    "mode=$(printf '%s\\n' \"$line\" | awk '{print substr($1,1,10)}') && "
    f"line=$(openssl dgst -sha256 {AP_CACHE_PATH}) && "
    "digest=$(printf '%s\\n' \"$line\" | awk '{print $NF}') && "
    "printf '%s %s\\n' \"$mode\" \"$digest\"; else printf 'missing\\n'; fi"
)
HOST_RE = re.compile(r"(?:[A-Za-z0-9_.-]+@)?[A-Za-z0-9][A-Za-z0-9_.:-]{0,252}\Z")


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


def publish(host: str, payload: bytes, ssh: str = "ssh", identity_file: str | None = None) -> None:
    """Atomically publish to the fixed AP cache, skipping identical content."""
    if not HOST_RE.fullmatch(host):
        raise ValueError("invalid AP SSH host")
    ssh_args = [ssh, "-T"]
    if identity_file:
        ssh_args.extend(["-i", identity_file, "-o", "IdentitiesOnly=yes"])
    ssh_args.extend(["-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=yes", "-o", "ConnectTimeout=8", host])
    expected_digest = hashlib.sha256(payload).hexdigest()
    if remote_state(ssh_args, host) == ("-rw-------", expected_digest):
        return
    try:
        result = subprocess.run(
            [*ssh_args, REMOTE_COMMAND],
            input=payload,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.PIPE,
            timeout=20,
            check=False,
        )
    except subprocess.TimeoutExpired as exc:
        raise RuntimeError(f"mapping publish timed out for {host}") from exc
    if result.returncode:
        raise RuntimeError(f"mapping publish failed for {host} (ssh exit {result.returncode})")
    if remote_state(ssh_args, host) != ("-rw-------", expected_digest):
        raise RuntimeError(f"mapping verification failed for {host}")
