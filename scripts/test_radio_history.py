#!/usr/bin/env python3
import importlib.util
import io
from contextlib import redirect_stdout
import ssl
import argparse
import json
import shlex
import subprocess
from pathlib import Path
import unittest
from unittest.mock import patch, mock_open
from urllib.error import URLError

SPEC = importlib.util.spec_from_file_location("radio_history", Path(__file__).parent / "ops" / "radio_history.py")
radio_history = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(radio_history)


class RadioHistoryTests(unittest.TestCase):
    def fixture(self, with_history=True):
        mac = "fc:34:97:46:7b:10"
        devices = {"data": [{
            "mac": mac,
            "radio_table": [{"name": "wifi-ng"}, {"name": "wifi-na"}, {"name": "wifi-na-1"}],
            "radio_table_stats": [
                {"name": "wifi-ng", "num_sta": 3, "bw": 20, "cu_total": 22},
                {"name": "wifi-na", "num_sta": 4, "bw": 40, "cu_total": 31},
                {"name": "wifi-na-1", "num_sta": 2, "bw": 80, "cu_total": 47},
            ],
            "vap_table": [{"radio_name": "wifi-na-1", "num_sta": 2, "avg_client_signal": -64,
                           "tx_success": 100, "tx_total": 102, "tx_retries": 9,
                           "wifi_tx_attempts": 111, "wifi_tx_dropped": 2}],
        }]}
        history = {"radio_details": []}
        if with_history:
            for name in ("wifi-ng", "wifi-na", "wifi-na-1"):
                history["radio_details"].append({"device_mac": mac, "radio_name": name,
                    "bytes": 0.0 if name == "wifi-na-1" else 123.0,
                    "client_signal_avg": 0.0 if name == "wifi-na-1" else -68.2,
                    "tx_retries_percentage": 0.0 if name == "wifi-na-1" else 3.1,
                    "interference_avg": 0.0})
        return devices, history

    def test_zero_history_with_current_upper_metrics_is_distinguished(self):
        devices, history = self.fixture()
        result = radio_history.project(devices, history)
        upper = next(row for row in result["main"] if row["name"] == "wifi-na-1")
        self.assertEqual(upper["history"], {"bytes": 0.0, "client_signal_avg": 0.0,
                         "tx_retries_percentage": 0.0, "interference_avg": 0.0})
        self.assertEqual(upper["live_vaps"], {"active": 1, "signal_measured": 1, "retry_counters_measured": 1})
        self.assertEqual(upper["num_sta"], 2)
        self.assertEqual(upper["bw"], 80)
        self.assertEqual(upper["observation"], "history_summary_zero_with_active_clients")
        self.assertNotIn("fc:34:97", str(result))
        self.assertNotIn("vap_table", str(result))

    def test_missing_history_stays_null_and_is_not_called_zero(self):
        devices, history = self.fixture(with_history=False)
        upper = next(row for row in radio_history.project(devices, history)["main"] if row["name"] == "wifi-na-1")
        self.assertEqual(upper["history"], {k: None for k in radio_history.HISTORY_FIELDS})
        self.assertNotIn("observation", upper)

    def test_wrong_context_stops_before_credential_read(self):
        output = io.StringIO()
        with patch.object(radio_history.socket, "gethostname", return_value="wrong-host"), patch("builtins.open", mock_open()) as opened, redirect_stdout(output):
            self.assertEqual(radio_history.remote(argparse.Namespace(insecure=False, ca_file=None)), 2)
        opened.assert_not_called()
        self.assertIn("unexpected CT107 execution context", output.getvalue())

    def test_remote_tls_is_verified_unless_explicitly_disabled(self):
        for insecure in (False, True):
            with self.subTest(insecure=insecure), patch.object(radio_history.socket, "gethostname", return_value="core-ops-broker"), patch("builtins.open", mock_open(read_data="private-key-fixture")), patch.object(radio_history, "fetch_json", side_effect=[{}, {}]) as fetch, patch.object(radio_history.ssl, "create_default_context") as verified, patch.object(radio_history.ssl, "_create_unverified_context") as unverified, redirect_stdout(io.StringIO()):
                self.assertEqual(radio_history.remote(argparse.Namespace(insecure=insecure, ca_file=None)), 0)
                self.assertEqual(verified.called, not insecure)
                self.assertEqual(unverified.called, insecure)
                self.assertEqual(fetch.call_count, 2)
                self.assertIn("?start=", fetch.call_args.args[0])
                self.assertIn("&end=", fetch.call_args.args[0])

    def test_ssh_target_and_ca_path_are_fixed_and_quoted(self):
        ca_path = "/etc/ca/test bundle;not-a-command.pem"
        result = subprocess.CompletedProcess([], 0, '{"main":[],"node":[]}', "private-error-fixture")
        output = io.StringIO()
        with patch.object(radio_history.sys, "argv", ["radio_history.py", "--ca-file", ca_path]), patch.object(radio_history.subprocess, "run", return_value=result) as run, redirect_stdout(output):
            self.assertEqual(radio_history.cli(), 0)
        command = run.call_args.args[0]
        self.assertEqual(command[-2], "root@10.100.1.100")
        self.assertIn("StrictHostKeyChecking=yes", command)
        self.assertEqual(shlex.split(command[-1]), ["/usr/sbin/pct", "exec", "107", "--", "/usr/bin/python3", "-", "--remote", "--ca-file", ca_path])
        self.assertNotIn("private-error-fixture", output.getvalue())

    def test_malformed_data_remains_unknown(self):
        self.assertEqual(radio_history.project({"data": None}, {"radio_details": None}), {"main": [], "node": []})
        devices, history = self.fixture()
        devices["data"].insert(0, {"mac": None})
        devices["data"][1]["radio_table_stats"][0].update(num_sta=True, bw=float("nan"), cu_total=101)
        history["radio_details"][0]["bytes"] = float("inf")
        row = radio_history.project(devices, history)["main"][0]
        self.assertIsNone(row["num_sta"])
        self.assertIsNone(row["bw"])
        self.assertIsNone(row["cu_total"])
        self.assertIsNone(row["history"]["bytes"])

    def test_transport_error_is_redacted(self):
        with patch("urllib.request.urlopen", side_effect=URLError("secret-api-key-and-url")):
            with self.assertRaises(RuntimeError) as caught:
                radio_history.fetch_json("/private?key=secret", ssl.create_default_context(), "secret-api-key")
        self.assertEqual(str(caught.exception), "controller transport or TLS failure")
        self.assertNotIn("secret", str(caught.exception))


if __name__ == "__main__":
    unittest.main()
