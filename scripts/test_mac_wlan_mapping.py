#!/usr/bin/env python3
"""Tests for Mac-to-CT107 lookup and Mac-to-AP publication orchestration."""

from __future__ import annotations

import json
import subprocess
import sys
import unittest
from pathlib import Path
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).parent / "ops"))
import mac_wlan_mapping  # noqa: E402


class MacWlanMappingTests(unittest.TestCase):
    mapping = {"blueberry": "0123456789abcdef01234567", "blueberry-prime": "abcdef0123456789abcdef01"}

    def test_remote_command_is_fixed_to_ct_key_and_exact_allowlist(self) -> None:
        command = mac_wlan_mapping.remote_command("https://10.100.1.1", "default", None, False)
        self.assertIn("/usr/sbin/pct exec 107", command)
        self.assertIn("/etc/ops-broker/unifi-api-key", command)
        self.assertIn("--ssid blueberry --ssid blueberry-prime", command)
        self.assertNotIn("api-key-value", command)
        with self.assertRaises(ValueError):
            mac_wlan_mapping.remote_command("https://10.100.1.1", "bad;site", None, False)

    def test_explicit_ssid_allowlist_is_quoted_deduplicated_and_exact(self) -> None:
        command = mac_wlan_mapping.remote_command(
            "https://10.100.1.1", "default", None, False, ["guest room", "guest room", "iot"]
        )
        self.assertIn("--ssid 'guest room' --ssid iot", command)
        requested = ["guest room", "iot"]
        mapping = {"guest room": "0123456789abcdef01234567", "iot": "abcdef0123456789abcdef01"}
        self.assertEqual(mac_wlan_mapping.validate_mapping(json.dumps(mapping).encode(), requested), mapping)
        with self.assertRaisesRegex(RuntimeError, "unexpected"):
            mac_wlan_mapping.validate_mapping(json.dumps({"guest room": mapping["guest room"]}).encode(), requested)
        with self.assertRaisesRegex(RuntimeError, "invalid mapping JSON"):
            mac_wlan_mapping.validate_mapping(b'{"iot":"0123456789abcdef01234567","iot":"abcdef0123456789abcdef01"}', ["iot"])

    def test_mapping_validation_rejects_extra_names_and_uuid_ids(self) -> None:
        with self.assertRaisesRegex(RuntimeError, "unexpected"):
            mac_wlan_mapping.validate_mapping(json.dumps({**self.mapping, "guest": "0123456789abcdef01234567"}).encode())
        invalid = {**self.mapping, "blueberry": "550e8400-e29b-41d4-a716-446655440000"}
        with self.assertRaisesRegex(RuntimeError, "ObjectId"):
            mac_wlan_mapping.validate_mapping(json.dumps(invalid).encode())
        duplicate_id = {"blueberry": "0123456789abcdef01234567", "blueberry-prime": "0123456789abcdef01234567"}
        with self.assertRaisesRegex(RuntimeError, "duplicate WLAN ObjectIds"):
            mac_wlan_mapping.validate_mapping(json.dumps(duplicate_id).encode())

    @patch.object(mac_wlan_mapping.subprocess, "run")
    def test_ct_lookup_streams_source_and_returns_only_mapping(self, run) -> None:  # noqa: ANN001
        run.return_value = subprocess.CompletedProcess([], 0, json.dumps(self.mapping).encode(), b"")
        result = mac_wlan_mapping.resolve_on_ct107("root@10.100.1.100", "https://10.100.1.1", "default", None, False)
        self.assertEqual(result, self.mapping)
        args, kwargs = run.call_args
        self.assertEqual(args[0][-2], "root@10.100.1.100")
        self.assertIn("StrictHostKeyChecking=yes", args[0])
        self.assertIn("/etc/ops-broker/unifi-api-key", args[0][-1])
        self.assertIn(b"def fetch_mapping", kwargs["input"])

    @patch.object(mac_wlan_mapping.subprocess, "run")
    def test_ct_lookup_passes_explicit_allowlist_and_checks_exact_response(self, run) -> None:  # noqa: ANN001
        requested = ["guest", "iot"]
        mapping = {"guest": "0123456789abcdef01234567", "iot": "abcdef0123456789abcdef01"}
        run.return_value = subprocess.CompletedProcess([], 0, json.dumps(mapping).encode(), b"")
        result = mac_wlan_mapping.resolve_on_ct107(
            "root@10.100.1.100", "https://10.100.1.1", "default", None, False, "ssh", requested
        )
        self.assertEqual(result, mapping)
        self.assertIn("--ssid guest --ssid iot", run.call_args.args[0][-1])

    @patch.object(mac_wlan_mapping.wlan_mapping_publish, "publish")
    @patch.object(mac_wlan_mapping, "resolve_on_ct107", return_value=mapping)
    def test_refresh_publishes_only_canonical_ids_to_both_aps(self, resolve, publish) -> None:  # noqa: ANN001
        mac_wlan_mapping.refresh("root@10.100.1.100", "https://10.100.1.1", "default", None, False, ["main", "node"])
        resolve.assert_called_once()
        self.assertEqual([call.args[0] for call in publish.call_args_list], ["main", "node"])
        payload = publish.call_args.args[1]
        self.assertEqual(json.loads(payload), self.mapping)
        self.assertNotIn(b"api-key", payload)

    @patch.object(mac_wlan_mapping.wlan_mapping_publish, "publish")
    @patch.object(mac_wlan_mapping, "resolve_on_ct107")
    def test_refresh_publishes_only_requested_names(self, resolve, publish) -> None:  # noqa: ANN001
        requested = ["guest", "iot"]
        mapping = {"guest": "0123456789abcdef01234567", "iot": "abcdef0123456789abcdef01"}
        resolve.return_value = mapping
        mac_wlan_mapping.refresh("root@10.100.1.100", "https://10.100.1.1", "default", None, False, ["main"], ssids=requested)
        self.assertEqual(resolve.call_args.args[-2:], ("ssh", requested))
        self.assertEqual(json.loads(publish.call_args.args[1]), mapping)


if __name__ == "__main__":
    unittest.main()
