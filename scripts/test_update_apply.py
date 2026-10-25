#!/usr/bin/env python3
"""Exercise immutable complete-bundle validation and updater failure paths."""
import hashlib
import io
import json
import os
from pathlib import Path
import subprocess
import tarfile
import tempfile
import time
import unittest

ROOT = Path(__file__).resolve().parent.parent
COMMIT = "a" * 40
MEMBERS = [
    "apply_config.sh", "bundle.json", "install.sh", "start.sh", "sync_nodes.sh",
    "unifi-bridge-armv7", "unifi-bridge.asp", "unifi-bridge.sh", "update.sh",
    "version.txt", "watchdog.sh",
]


class ApplyFailureTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.install = self.root / "addon"
        (self.install / "config").mkdir(parents=True)
        (self.install / "config/config.json").write_text('{"branch":"dev"}')
        (self.install / "version.txt").write_text("1.2.4\n")
        self.web = self.root / "www/unifi-bridge"
        self.web.mkdir(parents=True)
        self.bin = self.root / "bin"
        self.bin.mkdir()
        self.assets = self.root / "assets"
        self.assets.mkdir()
        (self.assets / "version.txt").write_text("1.2.5\n")
        self.env = dict(os.environ, PATH=str(self.bin) + os.pathsep + os.environ["PATH"], ASSETS=str(self.assets))
        self.requests = self.root / "requests.txt"
        self.env["REQUEST_LOG"] = str(self.requests)
        self.write(self.bin / "uname", "#!/bin/sh\necho armv7l\n")
        self.write(self.bin / "sleep", "#!/bin/sh\nexit 0\n")
        self.write(self.bin / "pidof", "#!/bin/sh\necho 1234\n")
        self.write(
            self.bin / "curl",
            "#!/usr/bin/env python3\n"
            "import os,sys,shutil,json\nfrom pathlib import Path\n"
            "a=sys.argv[1:]; url=next((v for v in a if v.startswith('https://')), '')\n"
            "with open(os.environ['REQUEST_LOG'],'a') as f: f.write(url+'\\n')\n"
            "if 'api.github.com' in url:\n"
            " print(json.dumps({'sha':os.environ.get('RESOLVED_COMMIT','" + COMMIT + "'),'tag_name':'v1.2.5'})); sys.exit(0)\n"
            "if url.endswith('/version.txt'):\n"
            " print((Path(os.environ['ASSETS'])/'version.txt').read_text(),end=''); sys.exit(0)\n"
            "name=url.split('?')[0].rsplit('/',1)[-1]; source=Path(os.environ['ASSETS'])/name\n"
            "if not source.exists(): sys.exit(22)\n"
            "shutil.copyfile(source,a[a.index('-o')+1])\n",
        )
        self.write_bundle()
        source = (ROOT / "scripts/update.sh").read_text()
        source = source.replace('INSTALL_DIR="/jffs/addons/$ADDON_NAME"', f'INSTALL_DIR="{self.install}"')
        source = source.replace("/tmp/unifi-bridge-update.lock", str(self.root / "update.lock"))
        source = source.replace("/tmp/var/wwwext", str(self.root / "www"))
        source = source.replace("/jffs/scripts", str(self.root / "links"))
        (self.root / "links").mkdir()
        self.script = self.root / "update.sh"
        self.script.write_text(source)

    def write(self, path, text):
        path.write_text(text)
        path.chmod(0o755)

    def write_bundle(self, *, version="1.2.5", commit=COMMIT, architecture="armv7", members=None, link=None, binary_commit=None, binary_exit=0):
        bundle_name = f"unifi-bridge-addon-{COMMIT}-armv7.tar.gz"
        self.bundle_name = bundle_name
        files = {
            "apply_config.sh": b"#!/bin/sh\nexit 0\n",
            "install.sh": b"#!/bin/sh\nexit 0\n",
            "start.sh": b"#!/bin/sh\nexit 0\n",
            "sync_nodes.sh": b"#!/bin/sh\nexit 0\n",
            "unifi-bridge-armv7": (
                f'#!/bin/sh\necho "unifi-bridge {version} (commit: {binary_commit or commit}, built: fixture)"\nexit {binary_exit}\n'
            ).encode(),
            "unifi-bridge.asp": b"candidate",
            "unifi-bridge.sh": b"#!/bin/sh\nexit 0\n",
            "update.sh": b"#!/bin/sh\nexit 0\n",
            "version.txt": (version + "\n").encode(),
            "watchdog.sh": b"#!/bin/sh\nexit 0\n",
            "bundle.json": json.dumps(
                {"version": version, "commit": commit, "architecture": architecture}, indent=2
            ).encode() + b"\n",
        }
        entries = list(MEMBERS if members is None else members)
        archive = self.assets / bundle_name
        with tarfile.open(archive, "w:gz", format=tarfile.USTAR_FORMAT) as tar:
            for name in entries:
                if name == link:
                    info = tarfile.TarInfo(name)
                    info.type = tarfile.SYMTYPE
                    info.linkname = "version.txt"
                    tar.addfile(info)
                elif name == "../escape":
                    info = tarfile.TarInfo(name)
                    info.size = 1
                    tar.addfile(info, io.BytesIO(b"x"))
                else:
                    data = files.get(name, b"unexpected")
                    info = tarfile.TarInfo(name)
                    info.mode = 0o755 if name.endswith(".sh") or name.startswith("unifi-bridge-arm") else 0o644
                    info.size = len(data)
                    tar.addfile(info, io.BytesIO(data))
        digest = hashlib.sha256(archive.read_bytes()).hexdigest()
        (self.assets / (bundle_name + ".sha256")).write_text(f"{digest}  {bundle_name}\n")

    def run_apply(self, env=None):
        return subprocess.run(
            ["sh", str(self.script), "apply", "dev", "fixture"],
            env=env or self.env,
            capture_output=True,
            text=True,
            timeout=8,
        )

    def assert_failure(self):
        result = self.run_apply()
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        status = json.loads((self.web / "update_status.js").read_text().split("=", 1)[1].strip().rstrip(";"))
        self.assertEqual(status["status"], "ERROR")
        self.assertFalse((self.root / "update.lock").exists(), "failed update leaked its lock")
        self.assertEqual((self.install / "version.txt").read_text(), "1.2.4\n")
        self.assertFalse((self.install / ".ssh").exists(), "invalid bundle mutated persistent install state")
        return status

    def test_missing_bundle_fails_closed(self):
        (self.assets / self.bundle_name).unlink()
        self.assert_failure()

    def test_bad_checksum_fails_closed(self):
        (self.assets / (self.bundle_name + ".sha256")).write_text("0" * 64 + "  " + self.bundle_name + "\n")
        self.assert_failure()

    def test_duplicate_manifest_entries_fail(self):
        path = self.assets / (self.bundle_name + ".sha256")
        line = path.read_text()
        path.write_text(line + line)
        self.assert_failure()

    def test_wrong_source_sha_fails(self):
        self.write_bundle(commit="b" * 40)
        self.assert_failure()

    def test_wrong_binary_sha_fails(self):
        self.write_bundle(binary_commit="b" * 40)
        self.assert_failure()

    def test_failed_version_command_fails_before_install_mutation(self):
        self.write_bundle(binary_exit=42)
        self.assert_failure()

    def test_wrong_version_fails(self):
        self.write_bundle(version="9.9.9")
        self.assert_failure()

    def test_wrong_architecture_fails(self):
        self.write_bundle(architecture="arm64")
        self.assert_failure()

    def test_missing_member_fails(self):
        self.write_bundle(members=[name for name in MEMBERS if name != "watchdog.sh"])
        self.assert_failure()

    def test_extra_and_duplicate_members_fail(self):
        self.write_bundle(members=MEMBERS + ["extra"])
        self.assert_failure()
        self.write_bundle(members=MEMBERS + ["version.txt"])
        self.assert_failure()

    def test_unsafe_path_and_link_members_fail(self):
        self.write_bundle(members=MEMBERS + ["../escape"])
        self.assert_failure()
        self.write_bundle(members=MEMBERS, link="version.txt")
        self.assert_failure()

    def test_unsupported_resolved_commit_fails_before_asset_lookup(self):
        result = self.run_apply(dict(self.env, RESOLVED_COMMIT="b" * 40))
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse((self.install / ".ssh").exists())
        self.assertFalse((self.root / "update.lock").exists())

    def test_resolved_commit_pins_asset_generation(self):
        # The mutable branch moves after API resolution; the exact old-SHA pair
        # remains the selected generation and must still be used.
        env = dict(self.env, RESOLVED_COMMIT=COMMIT)
        result = self.run_apply(env)
        self.assertNotEqual(result.returncode, 0)  # fixture start deliberately fails verification
        requests = self.requests.read_text()
        self.assertIn(f"/download/dev-{COMMIT}/{self.bundle_name}", requests)
        self.assertIn(f"/download/dev-{COMMIT}/{self.bundle_name}.sha256", requests)
        self.assertNotIn("/download/dev/", requests)
        self.assertEqual((self.install / "version.txt").read_text(), "1.2.5\n")

    def test_successful_disabled_install_uses_validated_generation(self):
        binary = (
            b'#!/bin/sh\ncase "$*" in *-version*) echo "unifi-bridge 1.2.5 (commit: '
            + COMMIT.encode()
            + b', built: fixture)";; *-is-enabled*) exit 3;; esac\n'
        )
        files = {name: b"#!/bin/sh\nexit 0\n" for name in MEMBERS}
        files["unifi-bridge-armv7"] = binary
        files["bundle.json"] = json.dumps(
            {"version": "1.2.5", "commit": COMMIT, "architecture": "armv7"}, indent=2
        ).encode() + b"\n"
        files["version.txt"] = b"1.2.5\n"
        archive = self.assets / self.bundle_name
        with tarfile.open(archive, "w:gz", format=tarfile.USTAR_FORMAT) as tar:
            for name in MEMBERS:
                data = files[name]
                info = tarfile.TarInfo(name)
                info.mode = 0o755
                info.size = len(data)
                tar.addfile(info, io.BytesIO(data))
        digest = hashlib.sha256(archive.read_bytes()).hexdigest()
        (self.assets / (self.bundle_name + ".sha256")).write_text(f"{digest}  {self.bundle_name}\n")
        result = self.run_apply()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        status = json.loads((self.web / "update_status.js").read_text().split("=", 1)[1].strip().rstrip(";"))
        self.assertEqual(status["status"], "SUCCESS")
        self.assertEqual(status["request_id"], "fixture")
        self.assertFalse((self.root / "update.lock").exists())


if __name__ == "__main__":
    unittest.main()
