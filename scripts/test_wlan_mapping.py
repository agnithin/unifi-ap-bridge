#!/usr/bin/env python3
"""Regression tests for the read-only WLAN mapping exporter."""

from __future__ import annotations

import json
import ssl
import sys
import unittest
from pathlib import Path
from unittest.mock import patch
from urllib.request import Request

sys.path.insert(0, str(Path(__file__).parent / "ops"))
import wlan_mapping  # noqa: E402


class FakeResponse:
    def __init__(self, value: object) -> None:
        self.raw = json.dumps(value).encode()

    def __enter__(self) -> "FakeResponse":
        return self

    def __exit__(self, *args: object) -> None:
        return None

    def read(self, size: int = -1) -> bytes:
        return self.raw if size < 0 else self.raw[:size]


class FakeOpener:
    def __init__(self, value: object) -> None:
        self.value = value
        self.request: Request | None = None
        self.timeout: int | None = None

    def open(self, request: Request, timeout: int) -> FakeResponse:
        self.request = request
        self.timeout = timeout
        return FakeResponse(self.value)


class WlanMappingTests(unittest.TestCase):
    rows = {
        "meta": {"rc": "ok"},
        "data": [
            {"name": "allowed", "_id": "6adb775a27327fe7fefdc22c", "external_id": "550e8400-e29b-41d4-a716-446655440000", "x_passphrase": "must-not-export"},
            {"name": "unrequested", "_id": "6adb775a27327fe7fefdc22d"},
        ],
    }

    def test_returns_only_exact_allowlist_and_legacy_id(self) -> None:
        result = wlan_mapping.resolve_mapping(self.rows, ["allowed"])
        self.assertEqual(result, {"allowed": "6adb775a27327fe7fefdc22c"})
        self.assertNotIn("external_id", json.dumps(result))
        self.assertNotIn("x_passphrase", json.dumps(result))

    def test_duplicate_missing_and_uuid_ids_fail_closed(self) -> None:
        duplicate = {"meta": {"rc": "ok"}, "data": self.rows["data"] + [self.rows["data"][0]]}
        with self.assertRaisesRegex(ValueError, "exactly one"):
            wlan_mapping.resolve_mapping(duplicate, ["allowed"])
        with self.assertRaisesRegex(ValueError, "exactly one"):
            wlan_mapping.resolve_mapping(self.rows, ["missing"])
        uuid_row = {"meta": {"rc": "ok"}, "data": [{"name": "allowed", "_id": "550e8400-e29b-41d4-a716-446655440000"}]}
        with self.assertRaisesRegex(ValueError, "legacy ObjectId"):
            wlan_mapping.resolve_mapping(uuid_row, ["allowed"])
        duplicate_id = {"meta": {"rc": "ok"}, "data": [
            {"name": "allowed", "_id": "0123456789abcdef01234567"},
            {"name": "other", "_id": "0123456789abcdef01234567"},
        ]}
        with self.assertRaisesRegex(ValueError, "duplicate legacy ObjectIds"):
            wlan_mapping.resolve_mapping(duplicate_id, ["allowed", "other"])

    def test_explicit_allowlist_is_deduplicated_and_bounded(self) -> None:
        self.assertEqual(wlan_mapping.normalize_ssids(["allowed", "allowed"]), ["allowed"])
        for invalid in ([], [""], ["x" * 33], ["\nname"]):
            with self.subTest(invalid=invalid), self.assertRaises(ValueError):
                wlan_mapping.normalize_ssids(invalid)
        too_many = [f"ssid-{i}" for i in range(wlan_mapping.MAX_REQUESTED_SSIDS + 1)]
        with self.assertRaisesRegex(ValueError, "exceeds"):
            wlan_mapping.normalize_ssids(too_many)

    def test_fetch_uses_fixed_get_endpoint_and_api_key_header(self) -> None:
        fake = FakeOpener(self.rows)
        with patch.object(wlan_mapping, "build_opener", return_value=fake):
            result = wlan_mapping.fetch_mapping("https://controller.test", "default", "fake-key", ["allowed"], ssl.create_default_context())
        self.assertEqual(result["allowed"], "6adb775a27327fe7fefdc22c")
        assert fake.request is not None
        self.assertEqual(fake.request.full_url, "https://controller.test/proxy/network/api/s/default/rest/wlanconf")
        self.assertEqual(fake.request.get_method(), "GET")
        self.assertEqual(fake.request.get_header("X-api-key"), "fake-key")


if __name__ == "__main__":
    unittest.main()
