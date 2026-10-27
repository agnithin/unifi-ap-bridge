#!/usr/bin/env python3
"""Export exact SSID -> legacy UniFi WLAN ObjectId mappings from a host.

This read-only helper calls one fixed classic WLAN-list endpoint. Keep its API
key on the host, pass an explicit SSID allowlist, and pipe the JSON result into
reviewed configuration tooling; it never changes AP/controller configuration.
"""

from __future__ import annotations

import argparse
import json
import re
import ssl
import sys
from pathlib import Path
from typing import Any
from urllib.error import HTTPError, URLError
from urllib.parse import quote, urlsplit, urlunsplit
from urllib.request import HTTPRedirectHandler, HTTPSHandler, Request, build_opener

MAX_RESPONSE_BYTES = 512 * 1024
MAX_RECORDS = 512
MAX_REQUESTED_SSIDS = 32
TIMEOUT_SECONDS = 8
SITE_RE = re.compile(r"[A-Za-z0-9_-]{1,64}\Z")


class RejectRedirects(HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):  # noqa: ANN001, ANN201
        raise HTTPError(req.full_url, code, "UniFi redirect rejected", headers, fp)


def valid_legacy_id(value: Any) -> bool:
    return isinstance(value, str) and re.fullmatch(r"[0-9a-fA-F]{24}", value) is not None


def normalize_ssids(requested_ssids: list[str]) -> list[str]:
    """Validate, deduplicate, and bound an explicit exact-name allowlist."""
    if not isinstance(requested_ssids, list) or not requested_ssids:
        raise ValueError("at least one non-empty SSID is required")
    if len(requested_ssids) > MAX_REQUESTED_SSIDS * 4:
        raise ValueError("SSID argument count exceeds the input bound")
    unique: list[str] = []
    seen: set[str] = set()
    for name in requested_ssids:
        if not isinstance(name, str) or not name:
            raise ValueError("SSID must contain 1 to 32 UTF-8 bytes")
        try:
            encoded_length = len(name.encode("utf-8"))
        except UnicodeEncodeError as exc:
            raise ValueError("SSID is not valid UTF-8 text") from exc
        if encoded_length > 32:
            raise ValueError("SSID must contain 1 to 32 UTF-8 bytes")
        if any(ord(char) < 32 or ord(char) == 127 for char in name):
            raise ValueError("SSID contains a control character")
        if name not in seen:
            unique.append(name)
            seen.add(name)
    if len(unique) > MAX_REQUESTED_SSIDS:
        raise ValueError(f"SSID allowlist exceeds {MAX_REQUESTED_SSIDS} unique names")
    return unique


def resolve_mapping(payload: Any, requested_ssids: list[str]) -> dict[str, str]:
    """Resolve an explicit exact-name allowlist to classic `_id` values."""
    wanted = set(normalize_ssids(requested_ssids))
    if isinstance(payload, dict):
        meta = payload.get("meta")
        if not isinstance(meta, dict) or meta.get("rc") != "ok":
            raise ValueError("UniFi did not report a successful WLAN list")
        rows = payload.get("data")
    elif isinstance(payload, list):
        rows = payload
    else:
        raise ValueError("unexpected WLAN list response")
    if not isinstance(rows, list) or len(rows) > MAX_RECORDS or any(not isinstance(row, dict) for row in rows):
        raise ValueError("WLAN list exceeded bounds or had invalid records")

    matched: dict[str, list[str]] = {name: [] for name in wanted}
    for row in rows:
        name = row.get("name")
        if isinstance(name, str) and name in matched:
            object_id = row.get("_id")
            if not valid_legacy_id(object_id):
                raise ValueError("a requested WLAN has no valid legacy ObjectId")
            matched[name].append(object_id)

    if any(len(ids) != 1 for ids in matched.values()):
        raise ValueError("each requested SSID must match exactly one WLAN")
    resolved = {name: ids[0] for name, ids in sorted(matched.items())}
    if len(set(resolved.values())) != len(resolved):
        raise ValueError("requested WLANs returned duplicate legacy ObjectIds")
    return resolved


def credential(path: Path) -> str:
    try:
        info = path.stat()
        value = path.read_text(encoding="utf-8").strip()
    except OSError as exc:
        raise ValueError("credential file is unavailable") from exc
    if not path.is_file() or info.st_mode & 0o077 or not value or len(value) > 1024 or any(c.isspace() for c in value):
        raise ValueError("credential file must be private and contain one API key")
    return value


def base_url(value: str) -> str:
    parsed = urlsplit(value)
    if parsed.scheme != "https" or not parsed.hostname or parsed.username or parsed.password or parsed.query or parsed.fragment:
        raise ValueError("controller URL must be an HTTPS origin")
    if parsed.path not in ("", "/"):
        raise ValueError("controller URL must not include a path")
    return urlunsplit(("https", parsed.netloc, "", "", ""))


def tls_context(ca_file: Path | None, insecure: bool) -> ssl.SSLContext:
    if ca_file is not None and insecure:
        raise ValueError("choose either a CA file or --insecure")
    if insecure:
        return ssl._create_unverified_context()  # noqa: S323 - explicit local-controller option
    return ssl.create_default_context(cafile=str(ca_file) if ca_file else None)


def fetch_mapping(
    controller: str,
    site: str,
    api_key: str,
    requested_ssids: list[str],
    context: ssl.SSLContext,
) -> dict[str, str]:
    requested = normalize_ssids(requested_ssids)
    if not SITE_RE.fullmatch(site):
        raise ValueError("site contains unsupported characters")
    root = base_url(controller)
    url = f"{root}/proxy/network/api/s/{quote(site, safe='')}/rest/wlanconf"
    request = Request(url, headers={"Accept": "application/json", "X-API-Key": api_key}, method="GET")
    opener = build_opener(RejectRedirects(), HTTPSHandler(context=context))
    try:
        with opener.open(request, timeout=TIMEOUT_SECONDS) as response:
            raw = response.read(MAX_RESPONSE_BYTES + 1)
    except HTTPError as exc:
        raise RuntimeError(f"UniFi returned HTTP {exc.code}") from exc
    except (URLError, TimeoutError, OSError) as exc:
        raise RuntimeError("UniFi request failed") from exc
    if len(raw) > MAX_RESPONSE_BYTES:
        raise RuntimeError("UniFi WLAN response exceeded the size limit")
    try:
        payload = json.loads(raw.decode("utf-8"))
    except (UnicodeDecodeError, json.JSONDecodeError) as exc:
        raise RuntimeError("UniFi returned invalid JSON") from exc
    return resolve_mapping(payload, requested)


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Read exact WLAN ObjectIds from UniFi and print an allowlisted JSON mapping.")
    parser.add_argument("--controller", required=True, help="UniFi controller HTTPS origin")
    parser.add_argument("--credential-file", required=True, type=Path, help="Private host-side file containing the API key")
    parser.add_argument("--site", default="default", help="Classic UniFi site name; default: default")
    parser.add_argument("--ssid", action="append", dest="ssids", help="Exact SSID to include; repeat for each allowed name (maximum 32 unique names)")
    tls = parser.add_mutually_exclusive_group()
    tls.add_argument("--ca-file", type=Path, help="CA bundle for controller TLS verification")
    tls.add_argument("--insecure", action="store_true", help="Disable TLS verification for a local controller with a self-signed certificate")
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    try:
        ssids = normalize_ssids(args.ssids or [])
        api_key = credential(args.credential_file)
        mapping = fetch_mapping(args.controller, args.site, api_key, ssids, tls_context(args.ca_file, args.insecure))
    except (ValueError, RuntimeError) as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        return 1
    print(json.dumps(mapping, ensure_ascii=False, sort_keys=True, separators=(",", ":")))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
