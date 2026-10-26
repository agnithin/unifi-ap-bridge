#!/usr/bin/env python3
"""Local regressions for durable foreground-command result capture."""
import json
import os
import pathlib
import subprocess
import sys
import tempfile
import time
import unittest

RUNNER = pathlib.Path(__file__).with_name("run_foreground.py")


class ForegroundCaptureTests(unittest.TestCase):
    def test_successful_exit_remains_zero(self):
        with tempfile.TemporaryDirectory(prefix="foreground-success-") as temporary:
            directory = pathlib.Path(temporary)
            directory.chmod(0o700)
            result, stdout, stderr = (directory / name for name in ("result.json", "stdout.log", "stderr.log"))
            run = subprocess.run([sys.executable, str(RUNNER), "--result", str(result), "--stdout", str(stdout),
                                  "--stderr", str(stderr), "--timeout-seconds", "5", "--", sys.executable,
                                  "-c", "pass"], capture_output=True, text=True, timeout=10)
            self.assertEqual(run.returncode, 0, run.stdout + run.stderr)
            record = json.loads(result.read_text())
            self.assertEqual(record["status"], "completed")
            self.assertEqual(record["client_exit_code"], 0)

    def test_exit_code_elapsed_time_and_logs_survive_child_return(self):
        with tempfile.TemporaryDirectory(prefix="foreground-capture-") as temporary:
            directory = pathlib.Path(temporary)
            directory.chmod(0o700)
            result, stdout, stderr = (directory / name for name in ("result.json", "stdout.log", "stderr.log"))
            started = time.monotonic()
            child = [sys.executable, "-c", "import sys,time;print('out');print('err',file=sys.stderr);time.sleep(.2);sys.exit(7)"]
            run = subprocess.run([sys.executable, str(RUNNER), "--result", str(result), "--stdout", str(stdout),
                                  "--stderr", str(stderr), "--timeout-seconds", "5", "--", *child],
                                 capture_output=True, text=True, timeout=10)
            self.assertEqual(run.returncode, 7, run.stdout + run.stderr)
            record = json.loads(result.read_text())
            self.assertEqual(record["status"], "completed")
            self.assertEqual(record["client_exit_code"], 7)
            self.assertGreaterEqual(record["elapsed_seconds"], .18)
            self.assertGreaterEqual(record["elapsed_seconds"], time.monotonic() - started - .1)
            self.assertEqual(stdout.read_text(), "out\n")
            self.assertEqual(stderr.read_text(), "err\n")
            for path in (result, stdout, stderr):
                self.assertEqual(path.stat().st_mode & 0o777, 0o600)

    def test_timeout_is_recorded_as_unknown_remote_completion(self):
        with tempfile.TemporaryDirectory(prefix="foreground-timeout-") as temporary:
            directory = pathlib.Path(temporary)
            directory.chmod(0o700)
            result, stdout, stderr = (directory / name for name in ("result.json", "stdout.log", "stderr.log"))
            child = [sys.executable, "-c", "import time;time.sleep(10)"]
            run = subprocess.run([sys.executable, str(RUNNER), "--result", str(result), "--stdout", str(stdout),
                                  "--stderr", str(stderr), "--timeout-seconds", ".1", "--", *child],
                                 capture_output=True, text=True, timeout=10)
            self.assertEqual(run.returncode, 124, run.stdout + run.stderr)
            record = json.loads(result.read_text())
            self.assertEqual(record["status"], "timed_out_remote_completion_unknown")
            self.assertIsNone(record["client_exit_code"])
            self.assertTrue(record["timed_out"])
            self.assertGreaterEqual(record["elapsed_seconds"], .1)

    def test_signal_termination_retains_raw_code_and_unknown_remote_completion(self):
        with tempfile.TemporaryDirectory(prefix="foreground-signal-") as temporary:
            directory = pathlib.Path(temporary)
            directory.chmod(0o700)
            result, stdout, stderr = (directory / name for name in ("result.json", "stdout.log", "stderr.log"))
            child = [sys.executable, "-c", "import os,signal;os.kill(os.getpid(),signal.SIGTERM)"]
            run = subprocess.run([sys.executable, str(RUNNER), "--result", str(result), "--stdout", str(stdout),
                                  "--stderr", str(stderr), "--timeout-seconds", "5", "--", *child],
                                 capture_output=True, text=True, timeout=10)
            self.assertEqual(run.returncode, 128 + 15, run.stdout + run.stderr)
            record = json.loads(result.read_text())
            self.assertEqual(record["status"], "terminated_by_signal_remote_completion_unknown")
            self.assertEqual(record["client_exit_code"], -15)
            self.assertEqual(record["termination_signal"], 15)
            self.assertFalse(record["timed_out"])


if __name__ == "__main__":
    unittest.main()
