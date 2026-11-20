#!/usr/bin/env python3
import unittest

from ops.wifi_health_probe import probe


class WiFiHealthProbeTests(unittest.TestCase):
    def test_checks_one_bssid_and_does_not_accept_dns_over_another_interface(self):
        for routed_interface, dns_expected in (("wlan0", True), ("eth0", False)):
            with self.subTest(routed_interface=routed_interface):
                commands = []

                def run(*args):
                    commands.append(args)
                    if args[0] == "iw":
                        return "Connected to 02:00:00:00:00:01 (on wlan0)"
                    if args[0] == "nmcli":
                        return "ip_address = 192.0.2.20"
                    if "addr" in args:
                        return '[{"addr_info":[{"local":"192.0.2.20"}]}]'
                    if "route" in args:
                        return '[{"dev":"' + routed_interface + '"}]'
                    self.assertEqual(args[0], "dig")
                    self.assertIn("192.0.2.20", args)
                    return "example.test. 60 IN A 192.0.2.30"

                expected = {"association": True, "dhcp_ipv4": True, "dns": dns_expected}
                self.assertEqual(probe("wlan0", "192.0.2.53", "example.test",
                                       "02:00:00:00:00:01", run), expected)
                self.assertEqual(any(cmd[0] == "dig" for cmd in commands), dns_expected)
                self.assertFalse(probe("wlan0", "192.0.2.53", "example.test",
                                       "02:00:00:00:00:02", run)["association"])


if __name__ == "__main__":
    unittest.main()
