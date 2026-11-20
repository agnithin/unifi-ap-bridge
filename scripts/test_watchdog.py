#!/usr/bin/env python3
"""Exercise watchdog health, stale-process guards and exponential backoff."""
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parent.parent


class WatchdogTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(dir="/tmp")
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.tmpfs = self.root / "tmp"
        self.install = self.root / "addon"
        self.bin = self.root / "bin"
        self.proc = self.root / "proc"
        self.status = self.tmpfs / "var/wwwext/unifi-bridge/status.js"
        self.config = self.install / "config/config.json"
        self.bin.mkdir()
        self.config.parent.mkdir(parents=True)
        self.config.write_text('{"enabled":true}')
        self.status.parent.mkdir(parents=True)
        self.proc.mkdir()
        self.alive = self.root / "alive"
        self.pidfile = self.root / "pid"
        self.launches = self.root / "launches"
        self.env = dict(os.environ, PATH=str(self.bin) + os.pathsep + os.environ["PATH"],
                        NOW="1000", PID_FILE=str(self.pidfile), ALIVE=str(self.alive),
                        PROC_ROOT=str(self.proc), LAUNCHES=str(self.launches))
        self.mock("pidof", 'cat "$PID_FILE" 2>/dev/null || true')
        self.mock("date", 'if [ "$1" = "+%s" ]; then echo "$NOW"; elif [ "$1" = "-r" ] && [ -n "${LOCK_MTIME:-}" ]; then echo "$LOCK_MTIME"; else echo test-time; fi')
        self.mock("sleep", 'exit 0')
        self.mock("kill", 'case "$1" in -0) [ -f "$ALIVE" ];; -TERM|-KILL) [ "${RESISTANT:-0}" = 1 ] || rm -f "$ALIVE";; esac')
        helper = self.tmpfs / "unifi-bridge/unifi-bridge"
        helper.parent.mkdir(parents=True)
        helper.write_text("#!/bin/sh\n[ \"$3\" = -is-enabled ] || exit 1\ngrep -q '\"enabled\"[[:space:]]*:[[:space:]]*false' \"$2\" && exit 3\nexit 0\n")
        helper.chmod(0o755)
        self.install.joinpath("start.sh").write_text('#!/bin/sh\necho launch >> "$LAUNCHES"\nexit "${START_RC:-0}"\n')
        self.install.joinpath("start.sh").chmod(0o755)
        source = (ROOT / "scripts/watchdog.sh").read_text()
        source = source.replace('INSTALL_DIR="/jffs/addons/$ADDON_NAME"', 'INSTALL_DIR="__INSTALL__"')
        source = source.replace('/tmp/unifi-bridge/unifi-bridge', '__HELPER__')
        source = source.replace('/bin/kill', '__KILL__')
        source = source.replace('/tmp/var/wwwext/unifi-bridge/status.js', '__STATUS_FILE__')
        source = source.replace('/tmp/', '__TMPROOT__/')
        source = source.replace('__TMPROOT__/', str(self.tmpfs) + '/')
        source = source.replace('__INSTALL__', str(self.install))
        source = source.replace('__STATUS_FILE__', str(self.status))
        source = source.replace('__HELPER__', str(helper))
        source = source.replace('__KILL__', str(self.bin / "kill"))
        source = source.replace('"/proc/$1/stat"', '"$PROC_ROOT/$1/stat"')
        self.script = self.root / "watchdog.sh"
        self.script.write_text(source)

    def mock(self, name, body):
        path = self.bin / name
        path.write_text("#!/bin/sh\n" + body + "\n")
        path.chmod(0o755)

    def run_watchdog(self):
        result = subprocess.run(["sh", str(self.script)], env=self.env,
                                capture_output=True, text=True, timeout=5)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        return result

    def process(self, pid="123", start="555"):
        (self.proc / pid).mkdir(exist_ok=True)
        tail = ["1"] * 18 + [start]
        (self.proc / pid / "stat").write_text(f"{pid} (unifi-bridge) S " + " ".join(tail))
        self.pidfile.write_text(pid)
        self.alive.write_text("yes")

    def test_missing_process_restart_uses_exponential_backoff(self):
        self.assertEqual(self.run_watchdog().returncode, 0)
        self.assertEqual(self.launches.read_text().count("launch"), 1)
        state = self.tmpfs / "unifi-bridge-watchdog-state"
        self.assertEqual(state.read_text().split(), ["1", "1300", "0"])
        self.env["NOW"] = "1100"
        self.run_watchdog()
        self.assertEqual(self.launches.read_text().count("launch"), 1)
        self.env["NOW"] = "1300"
        self.run_watchdog()
        self.assertEqual(self.launches.read_text().count("launch"), 2)
        self.assertEqual(state.read_text().split(), ["2", "1900", "0"])

    def test_fresh_health_and_disabled_config_never_restart(self):
        self.process()
        self.status.write_text('window.unifiBridgeStatus = {"pid":123,"sample_updated_at":990,"interval_sec":10,"sample_fresh":true};')
        self.run_watchdog()
        self.assertFalse(self.launches.exists())
        self.config.write_text('{"enabled":false}')
        self.pidfile.unlink()
        self.run_watchdog()
        self.assertFalse(self.launches.exists())

    def test_stale_same_process_gets_grace_then_targeted_recovery(self):
        self.process()
        self.status.write_text('window.unifiBridgeStatus = {"pid":123,"sample_updated_at":700,"interval_sec":10,"sample_fresh":false};')
        self.run_watchdog()
        observed = self.tmpfs / "unifi-bridge-watchdog-observed"
        self.assertTrue(observed.exists())
        self.env["NOW"] = "1180"
        self.run_watchdog()
        self.assertEqual(self.launches.read_text().count("launch"), 1)
        self.assertFalse(self.alive.exists())
        self.assertFalse(observed.exists())

    def test_pid_start_identity_change_restarts_observation_grace(self):
        self.process()
        self.status.write_text('window.unifiBridgeStatus = {"pid":123,"sample_updated_at":700,"interval_sec":10,"sample_fresh":false};')
        self.run_watchdog()
        self.env["NOW"] = "1180"
        self.process(start="556")
        self.run_watchdog()
        self.assertFalse(self.launches.exists())
        self.assertEqual((self.tmpfs / "unifi-bridge-watchdog-observed").read_text().split()[1], "556")

    def test_existing_mutation_guard_and_multiple_processes_refuse_recovery(self):
        (self.tmpfs / "unifi-bridge-mutation.lock").mkdir()
        self.assertEqual(self.run_watchdog().returncode, 0)
        self.assertFalse(self.launches.exists())
        (self.tmpfs / "unifi-bridge-mutation.lock").rmdir()
        self.pidfile.write_text("123 124")
        result = subprocess.run(["sh", str(self.script)], env=self.env, capture_output=True, text=True, timeout=5)
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(self.launches.exists())

    def test_term_resistant_stale_process_is_never_duplicated(self):
        self.process()
        self.status.write_text('window.unifiBridgeStatus = {"pid":123,"sample_updated_at":700,"interval_sec":10,"sample_fresh":false};')
        self.run_watchdog()
        self.env.update(NOW="1180", RESISTANT="1")
        result = subprocess.run(["sh", str(self.script)], env=self.env,
                                capture_output=True, text=True, timeout=5)
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(self.launches.exists())
        self.assertTrue(self.alive.exists())
        self.assertIn("refusing to start a duplicate", (self.tmpfs / "unifi-bridge-watchdog.log").read_text())

    def test_stale_start_lock_dead_pid_evicted_and_recovered(self):
        lock = self.tmpfs / "unifi-bridge-start.lock"
        lock.mkdir()
        (lock / "pid").write_text("99999\n")
        self.run_watchdog()
        self.assertEqual(self.launches.read_text().count("launch"), 1)
        self.assertFalse(lock.exists())
        self.assertIn("evicting stale lock", (self.tmpfs / "unifi-bridge-watchdog.log").read_text())

    def test_stale_mutation_lock_dead_pid_evicted_and_recovered(self):
        lock = self.tmpfs / "unifi-bridge-mutation.lock"
        lock.mkdir()
        (lock / "pid").write_text("88888\n")
        self.run_watchdog()
        self.assertEqual(self.launches.read_text().count("launch"), 1)
        self.assertFalse(lock.exists())
        self.assertIn("evicting stale lock", (self.tmpfs / "unifi-bridge-watchdog.log").read_text())

    def test_recent_incomplete_watchdog_lock_is_left_alone_then_reclaimed(self):
        lock = self.tmpfs / "unifi-bridge-watchdog.lock"
        lock.mkdir()
        self.env["LOCK_MTIME"] = "900"
        self.run_watchdog()
        self.assertFalse(self.launches.exists())
        self.env["NOW"] = "1501"
        self.run_watchdog()
        self.assertEqual(self.launches.read_text().count("launch"), 1)
        self.assertFalse(lock.exists())

    def test_recycled_watchdog_pid_does_not_block(self):
        self.process(start="555")
        lock = self.tmpfs / "unifi-bridge-watchdog.lock"
        lock.mkdir()
        (lock / "pid").write_text("123\n")
        (lock / "start").write_text("444\n")
        self.run_watchdog()
        self.assertFalse(lock.exists())
        self.assertIn("evicting stale lock", (self.tmpfs / "unifi-bridge-watchdog.log").read_text())

    def test_update_journal_reason_is_visible(self):
        transaction = self.install / ".update-transaction"
        transaction.mkdir()
        (transaction / "active").write_text("ACTIVATING\n")
        stale = self.tmpfs / "unifi-bridge-mutation.lock"
        stale.mkdir()
        (stale / "pid").write_text("99999\n")
        self.run_watchdog()
        self.assertFalse(stale.exists())
        self.assertTrue((transaction / "active").exists())
        self.assertIn("Update journal active", self.status.with_name("watchdog.js").read_text())
        self.assertFalse(self.launches.exists())

    def test_wlan_mapping_refresh_failure_preserves_previous_file(self):
        mapping_file = self.install / "config/wlan-mapping.json"
        mapping_file.write_text('{"old":"123"}')
        self.process()
        self.status.write_text('window.unifiBridgeStatus = {"pid":123,"sample_updated_at":990,"interval_sec":10,"sample_fresh":true};')
        self.run_watchdog()
        self.assertEqual(mapping_file.read_text(), '{"old":"123"}')
        self.assertIn("WLAN mapping refresh failed", self.status.with_name("watchdog.js").read_text())


if __name__ == "__main__":
    unittest.main()
