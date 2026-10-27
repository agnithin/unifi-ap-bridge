#!/usr/bin/env python3
"""Regression tests for the shared private AP mapping publisher."""

from __future__ import annotations

import hashlib
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).parent / "ops"))
import wlan_mapping_publish  # noqa: E402


class WlanMappingPublishTests(unittest.TestCase):
    def test_checksum_command_matches_ap_openssl_output_fixture(self) -> None:
        ap_fixture = "(stdin)= 9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"
        self.assertEqual(ap_fixture.split()[-1], hashlib.sha256(b"test").hexdigest())
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "mapping.json"
            path.write_bytes(b"test")
            path.chmod(0o600)
            command = wlan_mapping_publish.REMOTE_CHECKSUM_COMMAND.replace(
                wlan_mapping_publish.AP_CACHE_PATH, str(path)
            )
            result = subprocess.run(["/bin/sh", "-c", command], stdout=subprocess.PIPE, check=True)
        self.assertEqual(result.stdout.decode().strip(), f"-rw------- {hashlib.sha256(b'test').hexdigest()}")

    @patch.object(wlan_mapping_publish.subprocess, "run")
    def test_publish_is_fixed_atomic_private_and_verified(self, run) -> None:  # noqa: ANN001
        payload = b'{"blueberry":"0123456789abcdef01234567"}\n'
        digest = hashlib.sha256(payload).hexdigest().encode()
        run.side_effect = [
            subprocess.CompletedProcess([], 0, b"missing\n", b""),
            subprocess.CompletedProcess([], 0, b"", b""),
            subprocess.CompletedProcess([], 0, b"-rw------- " + digest + b"\n", b""),
        ]
        wlan_mapping_publish.publish("nithin@main-ap", payload)
        args, kwargs = run.call_args_list[1]
        self.assertEqual(args[0][-2], "nithin@main-ap")
        self.assertEqual(args[0][-1], wlan_mapping_publish.REMOTE_COMMAND)
        self.assertIn("StrictHostKeyChecking=yes", args[0])
        self.assertNotIn("IdentitiesOnly=yes", args[0])
        self.assertIn("chmod 600", wlan_mapping_publish.REMOTE_COMMAND)
        self.assertIn("mv -f", wlan_mapping_publish.REMOTE_COMMAND)
        self.assertEqual(kwargs["input"], payload)
        self.assertNotIn("api-key", kwargs["input"].decode())
        self.assertEqual(run.call_count, 3)

    @patch.object(wlan_mapping_publish.subprocess, "run")
    def test_unchanged_mapping_skips_flash_write(self, run) -> None:  # noqa: ANN001
        payload = b'{"blueberry":"0123456789abcdef01234567"}\n'
        digest = hashlib.sha256(payload).hexdigest().encode()
        run.return_value = subprocess.CompletedProcess([], 0, b"-rw------- " + digest + b"\n", b"")
        wlan_mapping_publish.publish("nithin@main-ap", payload)
        run.assert_called_once()

    def test_explicit_identity_is_pinned_when_provided(self) -> None:
        payload = b"{}\n"
        with patch.object(
            wlan_mapping_publish,
            "remote_state",
            return_value=("-rw-------", hashlib.sha256(payload).hexdigest()),
        ) as remote_state:
            wlan_mapping_publish.publish("nithin@main-ap", payload, identity_file="/private/key")
        args = remote_state.call_args.args[0]
        self.assertIn("/private/key", args)
        self.assertIn("IdentitiesOnly=yes", args)


if __name__ == "__main__":
    unittest.main()
