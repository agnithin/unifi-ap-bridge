#!/usr/bin/env python3
"""Regression tests for cluster SSH bounds and cached-status health checks."""
import json
import os
from pathlib import Path
import subprocess
import tempfile
import time
import unittest

SOURCE = (Path(__file__).parent / "sync_nodes.sh").read_text()

class ClusterTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.bin = self.root / "bin"
        self.bin.mkdir()
        self.env = dict(os.environ, PATH=str(self.bin) + os.pathsep + os.environ["PATH"])

    def stub(self, name, body):
        path = self.bin / name
        path.write_text("#!/bin/sh\n" + body + "\n")
        path.chmod(0o755)

    def test_watchdog_and_stdin(self):
        functions = SOURCE[SOURCE.index("cluster_ssh() ("):SOURCE.index("# Discover configured")]
        self.stub("ssh", "cat")
        result = subprocess.run(["sh", "-c", functions + '\ncluster_ssh node'],
                                input="archive bytes", capture_output=True, text=True,
                                env=self.env, timeout=3)
        self.assertEqual(result.returncode, 0)
        self.assertEqual(result.stdout, "archive bytes")
        self.stub("ssh", 'exec sleep 10')
        result = subprocess.run(["sh", "-c", functions + '\nCLUSTER_SSH_TIMEOUT=0.1 cluster_ssh node'],
                                capture_output=True, text=True, env=self.env, timeout=3)
        self.assertNotEqual(result.returncode, 0)

    def test_freshness(self):
        functions = SOURCE[SOURCE.index("status_is_fresh() {"):SOURCE.index("# Discover configured")]
        for now, updated, limit, expected in [
            (1000, 990, 60, 0), (1000, 900, 60, 1),
            (1000, 1001, 60, 1), (1000, "", 60, 1), (1000, "invalid", 60, 1),
        ]:
            result = subprocess.run(["sh", "-c", functions + '\nstatus_is_fresh "$1" "$2" "$3"',
                                     "test", str(now), str(updated), str(limit)])
            self.assertEqual(result.returncode, expected)

    def test_cluster_does_not_trust_cached_running(self):
        install = self.root / "install"
        (install / ".ssh").mkdir(parents=True)
        (install / ".ssh/cluster_key").touch()
        (install / "version.txt").write_text("1.2.2")
        self.stub("nvram", 'case "$2" in lan_ipaddr) echo 10.0.0.1;; lan_hostname) echo Main;; cfg_device_list) echo 10.0.0.2;; esac')
        self.stub("pidof", "echo 123")
        self.stub("ssh", 'cat "$FIXTURE"')
        source = SOURCE.replace('INSTALL_DIR="/jffs/addons/$ADDON_NAME"', 'INSTALL_DIR="' + str(install) + '"')
        source = source.replace('/tmp/var/wwwext', str(self.root / "www"))
        script = self.root / "sync.sh"
        script.write_text(source)
        fixture = self.root / "remote.txt"
        self.env["FIXTURE"] = str(fixture)
        for epoch, live_pid, expected in [(990, "456", True), (800, "456", False), (990, "", False), (None, "456", False)]:
            status = {"running": True, "enabled": True, "pid": 456, "commit": "a" * 40,
                      "version": "1.2.2", "hostname": "Node", "interval_sec": 10,
                      "total_stations": 3, "last_successful_inform": "2026-10-25 00:00:00",
                      "last_status": "Active (GCM)"}
            if epoch is not None:
                status["status_updated_at"] = epoch
            fixture.write_text('window.unifiBridgeStatus = ' + json.dumps(status, indent=2) + ';\n---LIVE---\n1000\n' + live_pid + '\n/tmp/unifi-bridge/unifi-bridge\n---VER---\n1.2.2\n---OP---\n---UPDATE---\n---TEMP---\n65000\n')
            subprocess.run(["sh", str(script), "status"], env=self.env, check=True, capture_output=True, timeout=3)
            raw = (self.root / "www/unifi-bridge/cluster.js").read_text()
            node = json.loads(raw.split("=", 1)[1].strip().rstrip(";"))["nodes"][1]
            self.assertEqual(node["status"] == "RUNNING", expected)
            self.assertEqual(node["sync_status"], "Version Match (Config Unverified)" if expected else "Stale or Stopped")


    def prepare_settings_sync(self, *, status_age=10, operation_status="APPLIED", dispatch_rc=0,
                              status_override=None, live_pid="", live_exe="", remote_unreachable=False):
        install = self.root / "install"
        (install / ".ssh").mkdir(parents=True)
        (install / ".ssh/cluster_key").touch()
        (install / "config").mkdir()
        (install / "config/config.json").write_text('{"enabled":false,"branch":"dev","interval_sec":15,"nodes":["10.0.0.2"],"uplink_remote_port":1}')
        web = self.root / "tmp/var/wwwext/unifi-bridge"
        web.mkdir(parents=True)
        commit = "a" * 40
        now = int(time.time())
        (web / "status.js").write_text(
            'window.unifiBridgeStatus = ' + json.dumps({"running": False, "enabled": False,
                "version": "1.2.5", "commit": commit, "status_updated_at": now,
                "interval_sec": 10, "pid": 123}) + ';\n')
        self.stub("nvram", 'case "$2" in lan_ipaddr) echo 10.0.0.1;; lan_hostname) echo Main;; esac')
        self.stub("pidof", "echo 123")
        query_status = {"running": False, "enabled": False, "version": "1.2.5", "commit": commit,
                        "status_updated_at": now - status_age, "interval_sec": 10,
                        "last_successful_inform": "2026-10-25 00:00:00", "last_status": "Active (GCM)"}
        if status_override:
            query_status.update(status_override)
        query = ('window.unifiBridgeStatus = ' + json.dumps(query_status) + ';\n'
                 '---LIVE---\n' + str(now) + '\n' + str(query_status.get("pid", "")) + '\n' + live_exe + '\n---VER---\n1.2.5\n---OP---\n'
                 + json.dumps({"operation_id": "123456", "operation": "sync", "status": operation_status})
                 + '\n---UPDATE---\n---TEMP---\n')
        fixture = self.root / "remote-query.txt"
        fixture.write_text(query)
        dispatch = self.root / "dispatch-input.txt"
        query_command = self.root / "remote-query-command.txt"
        ssh_body = ('exit 255' if remote_unreachable else
            'test ! -d "$LOCAL_MUTATION_LOCK" || exit 98\n'
            'last=""; for arg do last="$arg"; done\n'
            'case "$last" in *---LIVE---*) printf "%s" "$last" > "$REMOTE_QUERY_COMMAND"; cat "$REMOTE_QUERY";; *) cat > "$DISPATCH_INPUT"; exit "$DISPATCH_RC";; esac')
        self.stub("ssh", ssh_body)
        local_lock = self.root / "tmp/unifi-bridge-mutation.lock"
        self.env.update({"REMOTE_QUERY": str(fixture), "DISPATCH_INPUT": str(dispatch),
                         "REMOTE_QUERY_COMMAND": str(query_command), "DISPATCH_RC": str(dispatch_rc),
                         "LOCAL_MUTATION_LOCK": str(local_lock)})
        # Rewrite the source's fixed /tmp paths before injecting the fixture's
        # absolute install path; Linux temporary roots also begin with /tmp.
        source = SOURCE.replace('/tmp/', str(self.root / "tmp") + '/')
        source = source.replace('INSTALL_DIR="/jffs/addons/$ADDON_NAME"', 'INSTALL_DIR="' + str(install) + '"')
        script = self.root / "sync-settings.sh"
        script.write_text(source)
        return script, dispatch, commit

    def run_settings_sync(self, *, status_age=10, operation_status="APPLIED", dispatch_rc=0,
                          status_override=None, live_pid="", live_exe="", remote_unreachable=False):
        script, dispatch, commit = self.prepare_settings_sync(status_age=status_age,
            operation_status=operation_status, dispatch_rc=dispatch_rc,
            status_override=status_override, live_pid=live_pid, live_exe=live_exe,
            remote_unreachable=remote_unreachable)
        result = subprocess.run(["sh", str(script), "sync", "", "", "123456"], env=self.env,
            capture_output=True, text=True, timeout=5)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        raw = (self.root / "tmp/var/wwwext/unifi-bridge/cluster.js").read_text()
        try:
            nodes = json.loads(raw.split("=", 1)[1].strip().rstrip(";"))["nodes"]
        except Exception as exc:
            raise AssertionError(raw) from exc
        return nodes[1], dispatch, commit

    def test_sync_operation_is_verified_even_when_ssh_reply_is_lost_and_preserves_disabled_intent(self):
        node, dispatch, commit = self.run_settings_sync(dispatch_rc=1)
        self.assertEqual(node["operation_id"], "123456")
        self.assertEqual(node["dispatch_result"], "unknown")
        self.assertEqual(node["stage"], "verified")
        self.assertEqual(node["outcome"], "success")
        self.assertEqual(node["commit"], commit)
        self.assertEqual(node["last_operation_id"], "123456")
        self.assertEqual(node["last_operation_status"], "APPLIED")
        self.assertEqual(node["enabled"], False)
        self.assertIn('"enabled":false', dispatch.read_text().replace(" ", ""))
        main = json.loads((self.root / "tmp/var/wwwext/unifi-bridge/cluster.js").read_text().split("=", 1)[1].strip().rstrip(";"))["nodes"][0]
        self.assertEqual(main["enabled"], False)
        self.assertEqual(main["pid"], "123")
        self.assertEqual(main["outcome"], "unknown", "disabled metadata with a live PID is not healthy")
        query = (self.root / "remote-query-command.txt").read_text()
        self.assertIn("awk '{print $1}'", query, "remote shell must receive an unexpanded PID field reference")

    def test_sync_request_result_survives_routine_status_refresh(self):
        node, _, _ = self.run_settings_sync()
        result_path = self.root / "tmp/var/wwwext/unifi-bridge/operation_123456.js"
        original = result_path.read_bytes()
        operation = json.loads(original.decode().split("=", 1)[1].strip().rstrip(";"))
        self.assertEqual(operation["request_id"], "123456")
        self.assertEqual(operation["operation"], "sync")
        self.assertIn(operation["status"], ("UNKNOWN", "COMPLETED", "PARTIAL"))
        self.assertEqual(operation["nodes"][1]["operation_id"], "123456")
        subprocess.run(["sh", str(self.root / "sync-settings.sh"), "status"], env=self.env,
            check=True, capture_output=True, timeout=5)
        self.assertEqual(result_path.read_bytes(), original, "routine health refresh must not replace terminal request results")

    def test_sync_stale_status_remains_unknown(self):
        node, _, _ = self.run_settings_sync(status_age=500)
        self.assertEqual(node["freshness"], "stale")
        self.assertEqual(node["outcome"], "unknown")
        self.assertEqual(node["stage"], "verification_pending")
        self.assertEqual(node["last_operation_status"], "APPLIED")  # terminal result survives stale health

    def test_sync_explicit_remote_failure_is_per_node_failure(self):
        node, _, _ = self.run_settings_sync(operation_status="FAILED", dispatch_rc=1)
        self.assertEqual(node["outcome"], "failure")
        self.assertEqual(node["stage"], "failed")

    def test_applied_operation_with_fresh_stopped_enabled_node_is_not_success(self):
        node, _, _ = self.run_settings_sync(status_override={"running": False, "enabled": True,
            "pid": 123, "last_status": "Error: provider failed"}, live_exe="/tmp/unifi-bridge/unifi-bridge")
        self.assertEqual(node["freshness"], "fresh")
        self.assertEqual(node["stage"], "verification_pending")
        self.assertEqual(node["outcome"], "unknown")

    def test_unreachable_node_has_unknown_enabled_and_running_state(self):
        node, _, _ = self.run_settings_sync(remote_unreachable=True)
        self.assertEqual(node["stage"], "unreachable")
        self.assertEqual(node["outcome"], "unknown")
        self.assertIsNone(node["enabled"])
        self.assertIsNone(node["running"])

    def test_version_cmp(self):
        update_src = (Path(__file__).parent / "update.sh").read_text()
        func = update_src[update_src.index("version_cmp() {"):update_src.index("# Write update_status.js")]
        cases = [
            ("1.2.3", "1.2.3", 0),
            ("1.2.3", "v1.2.3", 0),
            ("1.2.3", "1.2.2", 1),
            ("1.2.3", "v1.2.1", 1),
            ("1.2.3", "1.3.0", 2),
            ("1.2.3", "2.0.0", 2),
            ("1.3.0", "1.2.9", 1),
            ("v1.2.3-dev", "v1.2.3", 0),
        ]
        for v1, v2, expected in cases:
            res = subprocess.run(["sh", "-c", func + '\nversion_cmp "$1" "$2"', "test", v1, v2])
            self.assertEqual(res.returncode, expected, f"Failed comparing {v1} and {v2}")

if __name__ == "__main__":
    unittest.main()
