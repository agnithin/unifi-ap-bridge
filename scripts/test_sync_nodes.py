#!/usr/bin/env python3
"""Regression tests for cluster SSH bounds and cached-status health checks."""
import json
import os
from pathlib import Path
import subprocess
import tempfile
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
            status = {"running": True, "pid": 456, "hostname": "Node", "interval_sec": 10, "total_stations": 3}
            if epoch is not None:
                status["status_updated_at"] = epoch
            fixture.write_text('window.unifiBridgeStatus = ' + json.dumps(status, indent=2) + ';\n---LIVE---\n1000\n' + live_pid + '\n---VER---\n1.2.2\n---TEMP---\n65000\n')
            subprocess.run(["sh", str(script), "status"], env=self.env, check=True, capture_output=True, timeout=3)
            raw = (self.root / "www/unifi-bridge/cluster.js").read_text()
            node = json.loads(raw.split("=", 1)[1].strip().rstrip(";"))["nodes"][1]
            self.assertEqual(node["status"] == "RUNNING", expected)
            self.assertEqual(node["sync_status"], "Version Match (Config Unverified)" if expected else "Stale or Stopped")

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
