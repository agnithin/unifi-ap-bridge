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
        source = source.replace('/tmp/unifi-bridge-mutation.lock', str(self.root / 'mutation.lock'))
        handler = self.root / "handler.sh"
        handler.write_text(source)
        subprocess.run(["sh",str(handler),"start","unifi_bridge_check_update_main_123456"],env=self.env,check=True,timeout=5)
        for _ in range(50):
            if capture.exists():break
            time.sleep(0.01)
        self.assertEqual(capture.read_text().splitlines(),["check","main","123456"])
        self.assertEqual(settings.read_text(), original)
        self.assertEqual(json.loads((self.install / 'config/config.json').read_text())["branch"], "dev")

    def settings_fixture(self, *, payload_id="123456", startup_rc=0, settings_result="SETTINGS_CHANGED=1"):
        self.install.mkdir(exist_ok=True)
        conf = self.install / "config"
        conf.mkdir(exist_ok=True)
        status_dir = self.root / "tmp/var/wwwext/unifi-bridge"
        status_dir.mkdir(parents=True, exist_ok=True)
        self.env.update({"AMNG": json.dumps({"request_id": payload_id, "enabled": True,
            "auto_gateway": False, "controller_url": "http://controller.test:8080/inform",
            "model": "U6IW", "interval": 10, "branch": "dev"}), "START_RC": str(startup_rc),
            "CAPTURE": str(self.root / "helper.json"), "HELPER_ARGS": str(self.root / "helper-args"),
            "SYNC_ARGS": str(self.root / "sync-args"), "SETTINGS_RESULT": settings_result,
            "START_CAPTURE": str(self.root / "start-args")})
        nvram = self.bin / "nvram"
        nvram.write_text('#!/bin/sh\ncase "$2" in amng_custom) printf "%s" "$AMNG";; lan_gateway) echo 10.0.0.1;; esac\n')
        nvram.chmod(0o755)
        bridge = self.root / "tmp/unifi-bridge/unifi-bridge"
        bridge.parent.mkdir(parents=True, exist_ok=True)
        bridge.write_text('#!/usr/bin/env python3\nimport json,os,sys\na=sys.argv[1:]; payload=sys.stdin.read(); cfg=a[a.index("-config")+1]\nexpected=a[a.index("-request-id")+1] if "-request-id" in a else ""\ndata=json.loads(payload)\nopen(os.environ["HELPER_ARGS"],"w").write(repr((a, data)))\nif expected and data.get("request_id") != expected: sys.exit(31)\ndata.pop("request_id",None)\nopen(os.environ["CAPTURE"],"w").write(json.dumps(data))\nopen(cfg,"w").write(json.dumps(data))\nprint(os.environ["SETTINGS_RESULT"])\n')
        bridge.chmod(0o755)
        (self.install / "start.sh").write_text('#!/bin/sh\ntouch "$START_CAPTURE"\nexit "$START_RC"\n')
        (self.install / "start.sh").chmod(0o755)
        (self.install / "sync_nodes.sh").write_text('#!/bin/sh\nprintf "%s\\n" "$@" > "$SYNC_ARGS"\n')
        (self.install / "sync_nodes.sh").chmod(0o755)
        source = (ROOT / "scripts/apply_config.sh").read_text()
        source = source.replace('/tmp/', str(self.root / 'tmp') + '/')
        source = source.replace('/jffs/addons/unifi-bridge', str(self.install))
        handler = self.root / "settings-handler.sh"
        handler.write_text(source)
        return handler, status_dir

    def run_settings(self, *, action_id="123456", payload_id="123456", startup_rc=0, settings_result="SETTINGS_CHANGED=1"):
        handler, status_dir = self.settings_fixture(payload_id=payload_id, startup_rc=startup_rc, settings_result=settings_result)
        result = subprocess.run(["sh", str(handler), "start", "start_unifi_bridge_apply_" + action_id],
            env=self.env, capture_output=True, text=True, timeout=5)
        status_path = status_dir / ("action_status_" + action_id + ".js")
        for _ in range(50):
            if status_path.exists() and (startup_rc != 0 or settings_result == "SETTINGS_CHANGED=0" or (self.root / "sync-args").exists()): break
            time.sleep(.02)
        status = json.loads(status_path.read_text().split("=",1)[1].strip().rstrip(";")) if status_path.exists() else None
        return result, status

    def test_settings_action_binds_payload_id_and_retains_per_request_completion(self):
        result, status = self.run_settings()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(status["request_id"], "123456")
        self.assertEqual(status["operation"], "settings")
        self.assertEqual(status["status"], "SAVED")
        self.assertNotIn("request_id", json.loads((self.root / "helper.json").read_text()))
        self.assertEqual((self.root / "sync-args").read_text().splitlines(), ["sync", "", "", "123456"])

    def test_superseded_settings_payload_is_rejected_without_config_write(self):
        prior_config = (self.install / "config/config.json").read_text()
        result, status = self.run_settings(action_id="123456", payload_id="654321")
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(status["status"], "REJECTED")
        self.assertEqual((self.install / "config/config.json").read_text(), prior_config)
        self.assertFalse((self.root / "sync-args").exists())

    def test_semantic_noop_save_does_not_start_service_or_sync_nodes(self):
        result, status = self.run_settings(settings_result="SETTINGS_CHANGED=0")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(status["status"], "SAVED_NOOP")
        self.assertFalse((self.root / "start-args").exists())
        self.assertFalse((self.root / "sync-args").exists())

    def test_start_failure_reports_settings_saved_separately(self):
        result, status = self.run_settings(startup_rc=1)
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(status["status"], "START_FAILED_AFTER_SAVE")
        self.assertTrue((self.install / "config/config.json").exists())

if __name__ == "__main__":
    unittest.main()
