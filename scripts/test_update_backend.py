#!/usr/bin/env python3
"""Run update-check and CGI dispatch code against temporary files and fake curl."""
import json
import os
from pathlib import Path
import subprocess
import tempfile
import time
import unittest

ROOT = Path(__file__).resolve().parent.parent

class UpdateBackendTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.install = self.root / "addon"
        (self.install / "config").mkdir(parents=True)
        self.status = self.root / "www"
        self.status.mkdir()
        self.bin = self.root / "bin"
        self.bin.mkdir()
        self.env = dict(os.environ, PATH=str(self.bin) + os.pathsep + os.environ["PATH"])
        (self.install / "version.txt").write_text("1.2.3\n")
        (self.install / "config/config.json").write_text('{"branch": "dev"}')
        (self.status / "status.js").write_text('window.unifiBridgeStatus = {\n  "branch": "dev"\n};\n')
        curl = self.bin / "curl"
        curl.write_text('#!/bin/sh\ncase "$*" in *latest*) printf "Location: https://example.test/v1.2.1\\r\\n";; *version.txt*) echo 1.2.3;; esac\n')
        curl.chmod(0o755)

    def check(self, branch):
        source = (ROOT / "scripts/update.sh").read_text().split("cmd_apply() {",1)[0]
        source = source.replace('INSTALL_DIR="/jffs/addons/$ADDON_NAME"', 'INSTALL_DIR="' + str(self.install) + '"')
        source = source.replace('/tmp/var/wwwext/$ADDON_NAME', str(self.status))
        script = self.root / "check.sh"
        script.write_text(source + '\ncmd_check "$1" "$2"\n')
        result = subprocess.run(["sh", str(script), branch, "123456"], env=self.env, capture_output=True, timeout=5)
        self.assertIn(result.returncode, (0, 1), result.stderr)
        raw = (self.status / "update_status.js").read_text()
        return json.loads(raw.split("=",1)[1].strip().rstrip(";"))

    def test_main_check_reports_normalized_versions_and_installed_track(self):
        result = self.check("main")
        self.assertEqual(result["current_version"], "1.2.3")
        self.assertEqual(result["remote_version"], "1.2.1")
        self.assertEqual(result["installed_branch"], "dev")
        self.assertEqual(result["branch"], "main")
        self.assertEqual(result["operation"], "check")
        self.assertEqual(result["request_id"], "123456")
        self.assertTrue(result["installed_newer"])
        self.assertIn("v1.2.3 (dev)", result["message"])
        self.assertIn("v1.2.1 (main)", result["message"])

    def test_same_version_dev_check(self):
        result = self.check("dev")
        self.assertEqual(result["status"], "UP_TO_DATE")
        self.assertEqual(result["remote_version"], "1.2.3")
        self.assertFalse(result["installed_newer"])

    def test_failure_retains_request_identity(self):
        (self.bin / "curl").write_text('#!/bin/sh\nexit 1\n')
        result = self.check("dev")
        self.assertEqual(result["status"], "ERROR")
        self.assertEqual(result["request_id"], "123456")

    def test_cgi_dispatch_preserves_track_and_passes_request_id(self):
        capture = self.root / "arguments"
        self.env["CAPTURE"] = str(capture)
        updater = self.install / "update.sh"
        updater.write_text('#!/bin/sh\nprintf "%s\\n" "$@" > "$CAPTURE"\n')
        updater.chmod(0o755)
        settings = self.root / "custom_settings.txt"
        original = 'unifibridge_branch dev\nother_addon_setting keep\n'
        settings.write_text(original)
        source = (ROOT / "scripts/apply_config.sh").read_text()
        source = source.replace('/jffs/addons/unifi-bridge', str(self.install))
        source = source.replace('/jffs/addons/custom_settings.txt', str(settings))
        source = source.replace('/tmp/unifi-bridge-update.log', str(self.root / 'update.log'))
        handler = self.root / "handler.sh"
        handler.write_text(source)
        subprocess.run(["sh",str(handler),"start","unifi_bridge_check_update_main_123456"],env=self.env,check=True,timeout=5)
        for _ in range(50):
            if capture.exists():break
            time.sleep(0.01)
        self.assertEqual(capture.read_text().splitlines(),["check","main","123456"])
        self.assertEqual(settings.read_text(), original)
        self.assertEqual(json.loads((self.install / 'config/config.json').read_text())["branch"], "dev")

if __name__ == "__main__":
    unittest.main()
