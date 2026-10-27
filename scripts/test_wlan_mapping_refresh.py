#!/usr/bin/env python3
"""Tests for the host-side bounded WLAN mapping publisher."""

from __future__ import annotations

import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).parent / "ops"))
import refresh_wlan_mapping  # noqa: E402


class WlanMappingRefreshTests(unittest.TestCase):
    def test_config_rejects_bad_hosts_and_duplicate_targets(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "config.json"
            for hosts in (["-oProxyCommand=bad"], ["ap1", "ap1"]):
                path.write_text(json.dumps({"controller": "https://controller.test", "ap_hosts": hosts}))
                with self.assertRaises(ValueError):
                    refresh_wlan_mapping.load_config(path)

    @patch.object(refresh_wlan_mapping.subprocess, "run")
    def test_publish_uses_fixed_private_atomic_destination(self, run) -> None:  # noqa: ANN001
        payload = b'{"blueberry":"0123456789abcdef01234567"}\n'
        digest = refresh_wlan_mapping.hashlib.sha256(payload).hexdigest().encode()
        run.side_effect = [
            subprocess.CompletedProcess([], 0, b"missing\n", b""),
            subprocess.CompletedProcess([], 0, b"", b""),
            subprocess.CompletedProcess([], 0, b"600 " + digest + b"\n", b""),
        ]
        refresh_wlan_mapping.publish("nithin@main-ap", payload, "/private/key")
        args, kwargs = run.call_args_list[1]
        self.assertEqual(args[0][-2], "nithin@main-ap")
        self.assertEqual(args[0][-1], refresh_wlan_mapping.REMOTE_COMMAND)
        self.assertIn("/private/key", args[0])
        self.assertIn("IdentitiesOnly=yes", args[0])
        self.assertIn("StrictHostKeyChecking=yes", args[0])
        self.assertIn("chmod 600", refresh_wlan_mapping.REMOTE_COMMAND)
        self.assertIn("mv -f", refresh_wlan_mapping.REMOTE_COMMAND)
        self.assertIn("/jffs/addons/unifi-bridge/config/wlan-mapping.json", refresh_wlan_mapping.REMOTE_COMMAND)
        self.assertEqual(kwargs["input"], payload)
        self.assertNotIn("unifi-api-key", kwargs["input"].decode())
        self.assertEqual(run.call_count, 3)

    @patch.object(refresh_wlan_mapping.subprocess, "run")
    def test_publish_skips_flash_write_when_sha256_matches(self, run) -> None:  # noqa: ANN001
        payload = b'{"blueberry":"0123456789abcdef01234567"}\n'
        digest = refresh_wlan_mapping.hashlib.sha256(payload).hexdigest().encode()
        run.return_value = subprocess.CompletedProcess([], 0, b"600 " + digest + b"\n", b"")
        refresh_wlan_mapping.publish("nithin@main-ap", payload, "/private/key")
        run.assert_called_once()

    @patch.object(refresh_wlan_mapping, "publish")
    @patch.object(refresh_wlan_mapping.wlan_mapping, "fetch_mapping")
    @patch.object(refresh_wlan_mapping.wlan_mapping, "credential", return_value="test-key")
    def test_refresh_sends_only_allowlisted_mapping_to_each_ap(self, credential, fetch, publish) -> None:  # noqa: ANN001
        fetch.return_value = {"blueberry": "0123456789abcdef01234567", "blueberry-prime": "abcdef0123456789abcdef01"}
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            config = root / "config.json"
            secret = root / "api-key"
            identity = root / "ssh-key"
            identity.write_text("private test key")
            identity.chmod(0o600)
            config.write_text(json.dumps({"controller": "https://controller.test", "ap_hosts": ["main-ap", "node-ap"], "identity_file": str(identity), "insecure": True}))
            secret.write_text("test-key")
            refresh_wlan_mapping.refresh(config, secret)
        fetch.assert_called_once()
        self.assertEqual(fetch.call_args.args[3], ["blueberry", "blueberry-prime"])
        self.assertEqual([call.args[0] for call in publish.call_args_list], ["main-ap", "node-ap"])
        self.assertEqual([call.args[2] for call in publish.call_args_list], [str(identity), str(identity)])
        payload = publish.call_args.args[1]
        self.assertIn(b"blueberry-prime", payload)
        self.assertNotIn(b"test-key", payload)
        self.assertNotIn("ca_file", fetch.call_args.kwargs)
        credential.assert_called_once()


if __name__ == "__main__":
    unittest.main()
