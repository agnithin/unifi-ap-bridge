#!/usr/bin/env python3
"""Disposable lifecycle tests for verified flat release bundles."""
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest
import tarfile

ROOT = Path(__file__).resolve().parent.parent
COMMIT = "b" * 40


class InstallLifecycleTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(dir="/tmp")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.source = self.root / "flat-release"
        self.source.mkdir()
        self.addon = self.root / "jffs/addons/unifi-bridge"
        self.scripts = self.root / "jffs/scripts"
        self.www = self.root / "tmp/var/wwwext"
        self.optbin = self.root / "opt/bin"
        self.menu = self.root / "tmp/menuTree.js"
        self.webmodule = self.root / "www/require/modules/menuTree.js"
        self.bin = self.root / "commands"
        self.bin.mkdir()
        self.kills = self.root / "killall.log"
        self.start_marker = self.root / "start.called"
        self.env = dict(os.environ, PATH=str(self.bin) + os.pathsep + os.environ["PATH"],
                        KILL_LOG=str(self.kills), START_MARKER=str(self.start_marker), GATEWAY="192.168.8.1")
        self.command("uname", 'echo "${ARCH:-armv7l}"')
        self.command("nvram", 'echo "$GATEWAY"')
        self.command("killall", 'echo "$*" >> "$KILL_LOG"; exit 0')
        for name in ("cru", "umount", "mount", "sync"):
            self.command(name, "exit 0")
        self.install = self.source / "install.sh"
        source = (ROOT / "scripts/install.sh").read_text()
        source = source.replace('INSTALL_DIR="/jffs/addons/$ADDON_NAME"', f'INSTALL_DIR="{self.addon}"')
        source = source.replace('SCRIPTS_DIR="/jffs/scripts"', f'SCRIPTS_DIR="{self.scripts}"')
        source = source.replace('TMP_DIR="/tmp/$ADDON_NAME"', f'TMP_DIR="{self.root}/tmp/unifi-bridge"')
        source = source.replace('/tmp/var/wwwext', str(self.www))
        source = source.replace('/opt/bin', str(self.optbin))
        self.install.write_text(source)
        self.install.chmod(0o755)
        self.write_bundle()
        self.scripts.mkdir(parents=True)
        self.scripts.joinpath("services-start").write_text(
            '#!/bin/sh\n# Other addon\n/opt/other/start.sh\n/opt/other/uses /jffs/addons/unifi-bridge/data\n')
        self.scripts.joinpath("services-stop").write_text(
            '#!/bin/sh\n# Other addon\nkillall other-daemon\n')
        self.scripts.joinpath("service-event").write_text(
            '#!/bin/sh\n# Other addon\nif test "$1" = other_addon; then /opt/other/apply.sh; fi\n')
        self.www.mkdir(parents=True)
        self.www.joinpath("user1.asp").write_text("other addon page")
        self.www.joinpath("user1.title").write_text("Other Addon")
        self.menu.write_text('menu = [{url: "user1.asp", tabName: "Other Addon"}, {url: "NULL"}]\n')

    def command(self, name, body):
        path = self.bin / name
        path.write_text("#!/bin/sh\n" + body + "\n")
        path.chmod(0o755)

    def write_bundle(self, *, arch="armv7", meta_arch="armv7", version="1.2.5", source_version=None,
                     checksum_damage=None, missing=None, missing_checksum=None, binary_commit=None, mixed=False):
        (self.source / "bundle.json").unlink(missing_ok=True)
        (self.source / "version.txt").write_text((source_version or version) + "\n")
        binary_name = "unifi-bridge-armv7" if arch == "armv7" else "unifi-bridge-arm64"
        files = {
            binary_name: ('#!/bin/sh\necho "unifi-bridge ' + version + ' (commit: ' + (binary_commit or COMMIT) + ', built: fixture)"\n').encode(),
            "install.sh": self.install.read_bytes(),
            "start.sh": b'#!/bin/sh\nprintf called > "$START_MARKER"\n',
            "apply_config.sh": b"#!/bin/sh\nexit 0\n",
            "sync_nodes.sh": b"#!/bin/sh\nexit 0\n",
            "unifi-bridge.sh": b"#!/bin/sh\nexit 0\n",
            "update.sh": b"#!/bin/sh\nexit 0\n",
            "watchdog.sh": b"#!/bin/sh\nexit 0\n",
            "unifi-bridge.asp": b"<!-- unifi-ap-bridge-webui fixture -->\n",
            "version.txt": (source_version or version).encode() + b"\n",
        }
        for name, body in files.items():
            path = self.source / name
            path.write_bytes(body)
            path.chmod(0o755 if name.endswith(".sh") or name.startswith("unifi-bridge-arm") else 0o644)
        metadata = {"version": version, "commit": COMMIT, "architecture": meta_arch}
        for name, body in files.items():
            metadata["sha256_" + name] = hashlib.sha256(body).hexdigest()
        if checksum_damage:
            metadata["sha256_" + checksum_damage] = "0" * 64
        if missing_checksum:
            metadata.pop("sha256_" + missing_checksum)
        (self.source / "bundle.json").write_text(json.dumps(metadata, indent=2) + "\n")
        (self.source / "bundle.json").chmod(0o644)
        if mixed:
            # Simulate one file carried over from a previous source generation
            # after the release metadata was produced.
            (self.source / "apply_config.sh").write_text("#!/bin/sh\necho other-generation\n")
        if missing:
            (self.source / missing).unlink(missing_ok=True)

    def run_install(self, expected=0):
        result = subprocess.run(["sh", str(self.install)], cwd=self.source, env=self.env,
                                capture_output=True, text=True, timeout=8)
        self.assertEqual(result.returncode == 0, expected == 0, result.stdout + result.stderr)
        return result

    def assert_preflight_untouched(self):
        self.assertFalse(self.kills.exists(), "existing service was stopped before package rejection")
        config = self.addon / "config/config.json"
        if config.exists():
            self.assertEqual(config.read_text(), '{"enabled":false,"preserve":"sentinel"}\n')
            self.assertEqual(sorted(str(p.relative_to(self.addon)) for p in self.addon.rglob("*")),
                             ["config", "config/config.json"])
        else:
            self.assertFalse(self.addon.exists(), "persistent addon files were written before rejection")

    def test_verified_flat_generation_installs_and_allocates_free_slot(self):
        self.run_install()
        self.assertEqual((self.addon / "version.txt").read_text(), "1.2.5\n")
        self.assertEqual((self.addon / ".user_slot").read_text(), "user2\n")
        self.assertTrue(self.start_marker.exists())
        archive = self.addon / "unifi-bridge.tar.gz"
        with tarfile.open(archive) as tar:
            self.assertEqual(sorted(tar.getnames()), sorted([
                "apply_config.sh", "bundle.json", "install.sh", "start.sh", "sync_nodes.sh",
                "unifi-bridge-armv7", "unifi-bridge.asp", "unifi-bridge.sh", "update.sh",
                "version.txt", "watchdog.sh",
            ]))
            self.assertEqual(tar.extractfile("unifi-bridge-armv7").read(), (self.source / "unifi-bridge-armv7").read_bytes())
        digest_line = (self.addon / "unifi-bridge.tar.gz.sha256").read_text().split()
        self.assertEqual(digest_line[0], hashlib.sha256(archive.read_bytes()).hexdigest())
        config = json.loads((self.addon / "config/config.json").read_text())
        self.assertEqual(config["controller_url"], "http://192.168.8.1:8080/inform")
        self.assertEqual(config["state_path"], str(self.addon / "state.json"))
        self.assertEqual((self.addon / "config/config.json").stat().st_mode & 0o777, 0o600)
        self.assertIn("/opt/other/start.sh", (self.scripts / "services-start").read_text())
        self.assertIn("/opt/other/uses /jffs/addons/unifi-bridge/data", (self.scripts / "services-start").read_text())
        self.assertIn("other-daemon", (self.scripts / "services-stop").read_text())
        self.assertIn("/opt/other/apply.sh", (self.scripts / "service-event").read_text())
        with tarfile.open(archive, "w:gz") as tar:
            tar.add(self.source / "version.txt", arcname="version.txt")
        self.run_install()
        with tarfile.open(archive) as tar:
            self.assertEqual(tar.extractfile("unifi-bridge-armv7").read(), (self.source / "unifi-bridge-armv7").read_bytes())

    def test_reject_checksum_missing_wrong_arch_mixed_generation_before_mutation(self):
        self.addon.mkdir(parents=True)
        (self.addon / "config").mkdir()
        protected = self.addon / "config/config.json"
        protected.write_text('{"enabled":false,"preserve":"sentinel"}\n')
        self.write_bundle(checksum_damage="start.sh")
        self.run_install(expected=1); self.assert_preflight_untouched()
        self.assertEqual(protected.read_text(), '{"enabled":false,"preserve":"sentinel"}\n')
        self.write_bundle(missing="watchdog.sh")
        self.run_install(expected=1); self.assert_preflight_untouched()
        self.write_bundle(meta_arch="arm64")
        self.run_install(expected=1); self.assert_preflight_untouched()
        self.write_bundle(source_version="1.2.4")
        self.run_install(expected=1); self.assert_preflight_untouched()
        self.write_bundle(mixed=True)
        self.run_install(expected=1); self.assert_preflight_untouched()
        self.write_bundle(binary_commit="c" * 40)
        self.run_install(expected=1); self.assert_preflight_untouched()

    def test_malformed_or_incomplete_metadata_fails_before_mutation(self):
        self.addon.mkdir(parents=True); (self.addon / "config").mkdir()
        protected = self.addon / "config/config.json"
        protected.write_text('{"enabled":false,"preserve":"sentinel"}\n')
        self.write_bundle(missing_checksum="install.sh")
        self.run_install(expected=1); self.assert_preflight_untouched()
        self.write_bundle()
        with (self.source / "bundle.json").open("a") as stream:
            stream.write('"unexpected": "extra"\n')
        self.run_install(expected=1); self.assert_preflight_untouched()

    def test_saved_slot_taken_by_another_addon_fails_before_mutation(self):
        self.addon.mkdir(parents=True)
        (self.addon / ".user_slot").write_text("user2\n")
        (self.www / "user2.asp").write_text("other addon page")
        self.run_install(expected=1)
        self.assertFalse(self.kills.exists())
        self.assertFalse((self.addon / "config").exists())
        self.assertEqual((self.www / "user2.asp").read_text(), "other addon page")

    def test_slot_recovery_prefers_a_fully_owned_pair_and_rejects_symlinks(self):
        owned = self.www / "user3.asp"
        owned.write_text("<!-- unifi-ap-bridge-webui -->")
        (self.www / "user3.title").write_text("UniFi AP Bridge")
        self.run_install()
        self.assertEqual((self.addon / ".user_slot").read_text(), "user3\n")

    def test_slot_scan_does_not_claim_marker_without_matching_title(self):
        (self.www / "user3.asp").write_text("<!-- unifi-ap-bridge-webui -->")
        (self.www / "user3.title").write_text("Other Addon")
        self.run_install()
        self.assertEqual((self.addon / ".user_slot").read_text(), "user2\n")
        self.assertEqual((self.www / "user3.asp").read_text(), "<!-- unifi-ap-bridge-webui -->")

    def test_slot_scan_rejects_symlink_even_when_target_looks_owned(self):
        target = self.root / "external-owned-looking-page"
        target.write_text("<!-- unifi-ap-bridge-webui -->")
        (self.www / "user3.asp").symlink_to(target)
        (self.www / "user3.title").write_text("UniFi AP Bridge")
        self.run_install()
        self.assertEqual((self.addon / ".user_slot").read_text(), "user2\n")
        self.assertEqual(target.read_text(), "<!-- unifi-ap-bridge-webui -->")

    def test_checkout_without_flat_release_metadata_and_exhausted_slots_fail_closed(self):
        # The supported source is the extracted flat release, not an arbitrary
        # repository checkout with mutable or absent build outputs.
        checkout = self.root / "checkout"
        checkout.mkdir()
        (checkout / "install.sh").write_text(self.install.read_text())
        result = subprocess.run(["sh", str(checkout / "install.sh")], env=self.env,
                                capture_output=True, text=True, timeout=5)
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(self.kills.exists())
        self.assertFalse(self.addon.exists())

        for i in range(1, 21):
            (self.www / f"user{i}.asp").write_text(f"other{i}")
        self.write_bundle()
        self.run_install(expected=1)
        self.assert_preflight_untouched()

    def test_unsupported_architecture_and_invalid_gateway_preserve_unknown(self):
        self.env["ARCH"] = "mips"
        self.run_install(expected=1); self.assert_preflight_untouched()
        self.env.pop("ARCH")
        self.env["GATEWAY"] = '1.2.3.4";touch /tmp/injected;#'
        self.run_install()
        config = json.loads((self.addon / "config/config.json").read_text())
        self.assertEqual(config["controller_url"], "")
        self.assertFalse((self.root / "injected").exists())

    def test_empty_gateway_does_not_invent_controller_address(self):
        self.env["GATEWAY"] = ""
        self.run_install()
        config = json.loads((self.addon / "config/config.json").read_text())
        self.assertEqual(config["controller_url"], "")

    def test_non_unicast_gateway_does_not_become_controller(self):
        for gateway in ("0.0.0.0", "127.0.0.1", "224.0.0.1", "255.255.255.255"):
            with self.subTest(gateway=gateway):
                self.env["GATEWAY"] = gateway
                self.run_install()
                config = json.loads((self.addon / "config/config.json").read_text())
                self.assertEqual(config["controller_url"], "")
                shutil.rmtree(self.addon)

    def test_disposable_uninstall_removes_only_owned_slot_menu_and_hooks(self):
        self.run_install()
        # Model the firmware page/menu published by startup.
        (self.www / "user2.asp").write_text("<!-- unifi-ap-bridge-webui fixture -->")
        (self.www / "user2.title").write_text("UniFi AP Bridge")
        self.menu.write_text('menu = [{url: "user1.asp", tabName: "Other Addon"}, {url: "user2.asp", tabName: "UniFi AP Bridge"}, {url: "NULL"}]\n')
        (self.addon / ".user_slot").unlink()
        cli = self.scripts / "unifi-bridge"
        uninstaller = self.root / "uninstall.sh"
        source = (ROOT / "scripts/uninstall.sh").read_text()
        source = source.replace('INSTALL_DIR="/jffs/addons/$ADDON_NAME"', f'INSTALL_DIR="{self.addon}"')
        source = source.replace('SCRIPTS_DIR=/jffs/scripts', f'SCRIPTS_DIR={self.scripts}')
        source = source.replace('WWW_DIR=/tmp/var/wwwext', f'WWW_DIR={self.www}')
        source = source.replace('/tmp/menuTree.js', str(self.menu)).replace('/www/require/modules/menuTree.js', str(self.webmodule))
        source = source.replace('/tmp/$ADDON_NAME', str(self.root / "tmp/unifi-bridge"))
        source = source.replace('/tmp/unifi-bridge.log', str(self.root / "tmp/unifi-bridge.log"))
        source = source.replace('/tmp/unifi-bridge-watchdog.log', str(self.root / "tmp/unifi-bridge-watchdog.log"))
        source = source.replace('/opt/bin/unifi-bridge', str(self.optbin / "unifi-bridge"))
        uninstaller.write_text(source); uninstaller.chmod(0o755)
        result = subprocess.run(["sh", str(uninstaller)], env=self.env, capture_output=True, text=True, timeout=8)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertFalse(self.addon.exists())
        self.assertFalse((self.www / "user2.asp").exists())
        self.assertTrue((self.www / "user1.asp").exists())
        self.assertIn('tabName: "Other Addon"', self.menu.read_text())
        self.assertNotIn('tabName: "UniFi AP Bridge"', self.menu.read_text())
        self.assertIn('/opt/other/start.sh', (self.scripts / "services-start").read_text())
        self.assertIn('/opt/other/uses /jffs/addons/unifi-bridge/data', (self.scripts / "services-start").read_text())
        self.assertIn('other-daemon', (self.scripts / "services-stop").read_text())
        self.assertIn('/opt/other/apply.sh', (self.scripts / "service-event").read_text())
        self.assertFalse(cli.exists())


if __name__ == "__main__":
    unittest.main()
