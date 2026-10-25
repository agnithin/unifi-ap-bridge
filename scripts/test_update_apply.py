#!/usr/bin/env python3
"""Exercise immutable complete-bundle validation and updater failure paths."""
import hashlib
import io
import json
import os
from pathlib import Path
import subprocess
import shutil
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
        (self.root / "tmp").mkdir()
        self.install = self.root / "addon"
        (self.install / "config").mkdir(parents=True)
        (self.install / "config/config.json").write_text('{"branch":"dev"}')
        (self.install / "config/state.json").write_text('{"adopted":true,"identity":"preserve"}')
        (self.install / ".ssh").mkdir()
        (self.install / ".ssh/cluster_key").write_text("private identity fixture")
        (self.install / "version.txt").write_text("1.2.4\n")
        self.web = self.root / "www/unifi-bridge"
        self.web.mkdir(parents=True)
        self.bin = self.root / "bin"
        self.bin.mkdir()
        self.assets = self.root / "assets"
        self.assets.mkdir()
        (self.assets / "version.txt").write_text("1.2.5\n")
        self.running = self.root / "running"
        self.env = dict(os.environ, PATH=str(self.bin) + os.pathsep + os.environ["PATH"], ASSETS=str(self.assets), STATUS_FILE=str(self.web / "status.js"), RUNNING=str(self.running))
        self.requests = self.root / "requests.txt"
        self.env["REQUEST_LOG"] = str(self.requests)
        self.env["UNIFI_BRIDGE_MUTATION_WAIT_SECONDS"] = "1"
        self.write(self.bin / "uname", "#!/bin/sh\necho armv7l\n")
        self.write(self.bin / "sleep", "#!/bin/sh\nexit 0\n")
        self.write(self.bin / "pidof", '#!/bin/sh\n[ -f "$RUNNING" ] && echo 1234\n')
        self.write(self.bin / "readlink", "#!/bin/sh\necho " + str(self.root / "tmp/unifi-bridge/unifi-bridge") + "\n")
        self.write(self.bin / "killall", '#!/bin/sh\nrm -f "$RUNNING"\nexit 0\n')
        self.write(self.bin / "nohup",
                   '#!/bin/sh\nidentity=$("$1" -version 2>/dev/null)\ncase "$identity" in\n'
                   '  "unifi-bridge 1.2.4"*) touch "$RUNNING"; cat > "$STATUS_FILE" <<EOF\n'
                   'window.unifiBridgeStatus = {"running": true, "enabled": true, "version": "1.2.4", "commit": "' + COMMIT + '", "pid": 1234, "status_updated_at": ' + '$(date +%s)' + ', "last_successful_inform": "2026-10-25 17:19:00", "last_status": "Active (GCM)"};\nEOF\n;;\n'
                   'esac\nexit 0\n')
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
        source = source.replace("/tmp/unifi-bridge-start.lock", str(self.root / "start.lock"))
        source = source.replace("/tmp/unifi-bridge-mutation.lock", str(self.root / "mutation.lock"))
        source = source.replace("/tmp/unifi-bridge/unifi-bridge", str(self.root / "tmp/unifi-bridge/unifi-bridge"))
        source = source.replace("/tmp/var/wwwext", str(self.root / "www"))
        source = source.replace("/jffs/scripts", str(self.root / "links"))
        (self.root / "links").mkdir()
        self.script = self.root / "update.sh"
        self.script.write_text(source)
        prior = self.install / "unifi-bridge.tar.gz"
        prior_binary = self.root / "unifi-bridge-armv7"
        self.write(prior_binary, '#!/bin/sh\ncase "$*" in *-version*) echo "unifi-bridge 1.2.4 (commit: ' + COMMIT + ', built: fixture)";; *-is-enabled*) exit 0;; esac\n')
        (self.web / "status.js").write_text(
            'window.unifiBridgeStatus = {"running": true, "enabled": true, "version": "1.2.4", "commit": "' + COMMIT + '", "pid": 1234, "status_updated_at": ' + str(int(time.time())) + ', "last_successful_inform": "2026-10-25 17:19:00", "last_status": "Active (GCM)"};\n'
        )
        self.running.touch()
        prior_start = self.install / "start.sh"
        self.write(prior_start,
                   '#!/bin/sh\ntouch "$RUNNING"\ncat > "$STATUS_FILE" <<EOF\n'
                   'window.unifiBridgeStatus = {"running": true, "enabled": true, "version": "1.2.4", "commit": "' + COMMIT + '", "pid": 1234, "status_updated_at": ' + '$(date +%s)' + ', "last_successful_inform": "2026-10-25 17:19:00", "last_status": "Active (GCM)"};\nEOF\n')
        for name in ("apply_config.sh", "sync_nodes.sh", "unifi-bridge.sh", "update.sh", "watchdog.sh"):
            self.write(self.install / name, "#!/bin/sh\nexit 0\n")
        (self.install / "webui").mkdir()
        (self.install / "webui/unifi-bridge.asp").write_text("old-ui")
        self.make_prior_bundle()
        runtime=self.root/"tmp/unifi-bridge";runtime.mkdir(parents=True)
        shutil.copyfile(prior_binary,runtime/"unifi-bridge")

    def write(self, path, text):
        path.write_text(text)
        path.chmod(0o755)

    def write_bundle(self, *, version="1.2.5", commit=COMMIT, architecture="armv7", members=None, link=None, binary_commit=None, binary_exit=0):
        bundle_name = f"unifi-bridge-addon-{COMMIT}-armv7.tar.gz"
        self.bundle_name = bundle_name
        files = {
            "apply_config.sh": b"#!/bin/sh\nexit 0\n",
            "install.sh": b"#!/bin/sh\nexit 0\n",
            "start.sh": self.start_source().encode(),
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

    def make_prior_bundle(self):
        files={
            "apply_config.sh":(self.install/"apply_config.sh").read_bytes(),
            "bundle.json":json.dumps({"version":"1.2.4","commit":COMMIT,"architecture":"armv7"},indent=2).encode()+b"\n",
            "install.sh":b"#!/bin/sh\nexit 0\n",
            "start.sh":(self.install/"start.sh").read_bytes(),
            "sync_nodes.sh":(self.install/"sync_nodes.sh").read_bytes(),
            "unifi-bridge-armv7":(self.root/"unifi-bridge-armv7").read_bytes(),
            "unifi-bridge.asp":(self.install/"webui/unifi-bridge.asp").read_bytes(),
            "unifi-bridge.sh":(self.install/"unifi-bridge.sh").read_bytes(),
            "update.sh":(self.install/"update.sh").read_bytes(),
            "version.txt":b"1.2.4\n",
            "watchdog.sh":(self.install/"watchdog.sh").read_bytes(),
        }
        archive=self.install/"unifi-bridge.tar.gz"
        with tarfile.open(archive,"w:gz",format=tarfile.USTAR_FORMAT) as tar:
            for name,data in files.items():
                info=tarfile.TarInfo(name);info.mode=0o755 if name.endswith(".sh") or name.startswith("unifi-bridge-arm") else 0o644;info.size=len(data);tar.addfile(info,io.BytesIO(data))
        digest=hashlib.sha256(archive.read_bytes()).hexdigest()
        (self.install/"unifi-bridge.tar.gz.sha256").write_text(f"{digest}  unifi-bridge.tar.gz\n")

    def start_source(self):
        source = (ROOT / "scripts/start.sh").read_text()
        return (source.replace('/jffs/addons/unifi-bridge', str(self.install))
                .replace('/tmp/unifi-bridge-start.lock', str(self.root / 'start.lock'))
                .replace('/tmp/unifi-bridge-mutation.lock', str(self.root / 'mutation.lock'))
                .replace('/tmp/', str(self.root / 'tmp') + '/'))

    def run_apply(self, env=None, args=None):
        return subprocess.run(
            ["sh", str(self.script), *(args or ["apply", "dev", "fixture"])],
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
        self.assertEqual((self.install / ".ssh/cluster_key").read_text(), "private identity fixture", "invalid bundle changed SSH identity")
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

    def test_busy_mutation_lock_is_bounded_and_never_stolen(self):
        lock = self.root / "mutation.lock"
        lock.mkdir()
        (lock / "pid").write_text("424242\n")
        original_version = (self.install / "version.txt").read_text()
        result = self.run_apply()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("Mutation lock busy", result.stdout + result.stderr)
        self.assertEqual((lock / "pid").read_text(), "424242\n")
        self.assertEqual((self.install / "version.txt").read_text(), original_version)
        self.assertFalse((self.install / ".update-transaction").exists())

    def test_apply_exact_uses_sha_qualified_release(self):
        result = self.run_apply(args=["apply-exact", "dev", COMMIT, "777123"])
        self.assertNotEqual(result.returncode, 0)  # normal fixture startup rejects the candidate
        requests = self.requests.read_text()
        self.assertIn(f"/download/dev-{COMMIT}/{self.bundle_name}", requests)
        self.assertNotIn("/commits/dev", requests)
        operation = json.loads((self.web / "cluster_operation.js").read_text().split("=", 1)[1].strip().rstrip(";"))
        self.assertEqual(operation["operation_id"], "777123")
        self.assertEqual(operation["operation"], "update")
        self.assertEqual(operation["status"], "FAILED")
        self.assertEqual((self.install / "version.txt").read_text(), "1.2.4\n")
        self.assertFalse((self.root / "mutation.lock").exists())

    def test_cluster_receiver_update_does_not_propagate_recursively(self):
        source = (ROOT / "scripts/update.sh").read_text()
        start = source.index("should_propagate_update() {")
        end = source.index("\n}", start) + 2
        function = source[start:end]
        for operation, expected in (("cluster_update", 1), ("apply", 0)):
            result = subprocess.run(["sh", "-c", function + '\nshould_propagate_update', "test"],
                env=dict(self.env, operation=operation), capture_output=True, timeout=2)
            self.assertEqual(result.returncode, expected, operation)

    def test_apply_exact_main_requires_matching_stable_commit(self):
        result = self.run_apply(args=["apply-exact", "main", COMMIT, "777124"])
        self.assertNotEqual(result.returncode, 0)  # candidate launch fails in this fixture
        requests = self.requests.read_text()
        self.assertIn(f"/download/v1.2.5/{self.bundle_name}", requests)
        self.assertFalse((self.install / ".update-transaction").exists())

        self.requests.write_text("")
        mismatch = dict(self.env, RESOLVED_COMMIT="b" * 40)
        result = self.run_apply(env=mismatch, args=["apply-exact", "main", COMMIT, "777125"])
        self.assertNotEqual(result.returncode, 0)
        self.assertNotIn(f"/download/v1.2.5/{self.bundle_name}", self.requests.read_text())

    def test_unsupported_resolved_commit_fails_before_asset_lookup(self):
        result = self.run_apply(dict(self.env, RESOLVED_COMMIT="b" * 40))
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual((self.install / ".ssh/cluster_key").read_text(), "private identity fixture")
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
        self.assertEqual((self.install / "version.txt").read_text(), "1.2.4\n")
        update_status = json.loads((self.web / "update_status.js").read_text().split("=", 1)[1].strip().rstrip(";"))
        self.assertFalse((self.install / ".update-transaction").exists(), update_status["message"])
        self.assertIn("restored and verified", update_status["message"])
        self.assertEqual((self.install / "config/config.json").read_text(), '{"branch":"dev"}')
        self.assertEqual((self.install / "config/state.json").read_text(), '{"adopted":true,"identity":"preserve"}')
        self.assertEqual((self.install / ".ssh/cluster_key").read_text(), "private identity fixture")

    def test_active_asset_copy_failure_rolls_back_prior_generation(self):
        self.write(self.bin / "cp", "#!/usr/bin/env python3\nimport os,sys,subprocess\n\nif any('apply_config.sh.update.' in x for x in sys.argv): sys.exit(1)\nsys.exit(subprocess.call(['/bin/cp']+sys.argv[1:]))\n")
        prior_archive = (self.install / "unifi-bridge.tar.gz").read_bytes()
        prior_start = (self.install / "start.sh").read_text()
        result = self.run_apply()
        self.assertNotEqual(result.returncode, 0)
        status = json.loads((self.web / "update_status.js").read_text().split("=", 1)[1].strip().rstrip(";"))
        self.assertIn("restored and verified", status["message"],result.stdout+result.stderr)
        self.assertEqual((self.install / "version.txt").read_text(), "1.2.4\n")
        self.assertEqual((self.install / "unifi-bridge.tar.gz").read_bytes(), prior_archive)
        self.assertEqual((self.install / "start.sh").read_text(), prior_start)
        self.assertFalse((self.install / ".update-transaction").exists())

    def test_enabled_candidate_requires_fresh_identity_then_commits(self):
        self.write(self.bin / "nohup",
                   '#!/bin/sh\ntouch "$RUNNING"\ncat > "$STATUS_FILE" <<EOF\n'
                   'window.unifiBridgeStatus = {"running": true, "enabled": true, "version": "1.2.5", "commit": "' + COMMIT + '", "pid": 1234, "status_updated_at": ' + '$(date +%s)' + ', "last_successful_inform": "2026-10-25 17:20:00", "last_status": "Active (GCM)"};\nEOF\n')
        result=self.run_apply()
        self.assertEqual(result.returncode,0,result.stdout+result.stderr)
        status=json.loads((self.web/"update_status.js").read_text().split("=",1)[1].strip().rstrip(";"))
        self.assertEqual(status["status"],"SUCCESS")
        self.assertEqual((self.install/"version.txt").read_text(),"1.2.5\n")
        self.assertFalse((self.install/".update-transaction").exists())
        self.assertEqual((self.install/"config/config.json").read_text(),'{"branch":"dev"}')
        self.assertEqual((self.install/"config/state.json").read_text(),'{"adopted":true,"identity":"preserve"}')
        self.assertEqual((self.install/".ssh/cluster_key").read_text(),"private identity fixture")
        installed_manifest = (self.install / "unifi-bridge.tar.gz.sha256").read_text()
        self.assertRegex(installed_manifest, r"^[0-9a-f]{64}  unifi-bridge\.tar\.gz\n$")

        # A second activation must accept the first activation as its complete,
        # checksum-verified rollback generation. Fail its first asset replacement
        # and require recovery to relaunch that just-installed generation.
        fail_marker = self.root / "copy-failed-once"
        self.write(self.bin / "cp",
                   "#!/usr/bin/env python3\nimport os,sys,subprocess\nfrom pathlib import Path\n"
                   f"marker=Path({str(fail_marker)!r})\n"
                   "if any('apply_config.sh.update.' in x for x in sys.argv) and not marker.exists():\n"
                   " marker.touch(); sys.exit(1)\n"
                   "sys.exit(subprocess.call(['/bin/cp']+sys.argv[1:]))\n")
        second = self.run_apply()
        second_status = json.loads((self.web / "update_status.js").read_text().split("=", 1)[1].strip().rstrip(";"))
        self.assertNotEqual(second.returncode, 0)
        self.assertIn("restored and verified", second_status["message"], second.stdout + second.stderr)
        self.assertEqual((self.install / "version.txt").read_text(), "1.2.5\n")
        self.assertEqual((self.install / "unifi-bridge.tar.gz.sha256").read_text(), installed_manifest)
        self.assertFalse((self.install / ".update-transaction").exists())

    def test_disabled_candidate_copy_failure_restores_disabled_generation(self):
        (self.install/"config/config.json").write_text('{"branch":"dev","enabled":false,"unknown":"keep"}')
        self.write(self.bin/"pidof","#!/bin/sh\nexit 0\n")
        prior_binary=self.root/"unifi-bridge-armv7"
        self.write(prior_binary,
                   '#!/bin/sh\ncase "$*" in *-version*) echo "unifi-bridge 1.2.4 (commit: '+COMMIT+', built: fixture)";; '
                   '*-is-enabled*) exit 3;; *-once*) cat > "$STATUS_FILE" <<EOF\n'
                   'window.unifiBridgeStatus = {"running":false,"enabled":false,"version":"1.2.4","commit":"'+COMMIT+'","status_updated_at":' + '$(date +%s)' + '};\nEOF\n;; esac\n')
        self.make_prior_bundle();shutil.copyfile(prior_binary,self.root/"tmp/unifi-bridge/unifi-bridge")
        (self.web/"status.js").write_text('window.unifiBridgeStatus = {"running":false,"enabled":false,"version":"1.2.4","commit":"'+COMMIT+'","status_updated_at":'+str(int(time.time()))+'};')
        self.write(self.bin/"cp","#!/usr/bin/env python3\nimport sys,subprocess\nif any('apply_config.sh.update.' in x for x in sys.argv):sys.exit(1)\nsys.exit(subprocess.call(['/bin/cp']+sys.argv[1:]))\n")
        result=self.run_apply()
        self.assertNotEqual(result.returncode,0)
        status=json.loads((self.web/"update_status.js").read_text().split("=",1)[1].strip().rstrip(";"))
        self.assertIn("restored and verified",status["message"])
        self.assertEqual((self.install/"version.txt").read_text(),"1.2.4\n")
        self.assertFalse((self.install/".update-transaction").exists())

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
        (self.install / "config/config.json").write_text('{"branch":"dev","enabled":false,"unknown":"keep"}')
        self.write(self.bin / "pidof", "#!/bin/sh\nexit 0\n")
        prior_binary = self.root / "unifi-bridge-armv7"
        self.write(prior_binary,
                   '#!/bin/sh\ncase "$*" in *-version*) echo "unifi-bridge 1.2.4 (commit: ' + COMMIT + ', built: fixture)";; *-is-enabled*) exit 3;; esac\n')
        with tarfile.open(self.install / "unifi-bridge.tar.gz", "w:gz") as tar:
            pass
        self.make_prior_bundle()
        shutil.copyfile(prior_binary,self.root/"tmp/unifi-bridge/unifi-bridge")
        # Disabled status is static; an hours-old identity remains valid when
        # the installed helper confirms disabled and no daemon process exists.
        (self.web / "status.js").write_text('window.unifiBridgeStatus = {"running":false,"enabled":false,"version":"1.2.4","commit":"' + COMMIT + '","status_updated_at":'+str(int(time.time()) - 4 * 60 * 60)+'};\n')
        disabled = self.root / "disabled-status.js"
        start_script = self.root / "disabled-start.sh"
        self.write(start_script,
                   '#!/bin/sh\ncat > "' + str(disabled) + '" <<EOF\n'
                   'window.unifiBridgeStatus = {"running":false,"enabled":false,"version":"1.2.5","commit":"' + COMMIT + '","status_updated_at":$(date +%s)};\nEOF\n'
                   'cp "' + str(disabled) + '" "' + str(self.web / 'status.js') + '"\n')
        with tarfile.open(archive, "w:gz", format=tarfile.USTAR_FORMAT) as tar:
            for name in MEMBERS:
                data = start_script.read_bytes() if name == "start.sh" else files[name]
                info = tarfile.TarInfo(name); info.mode = 0o755; info.size = len(data); tar.addfile(info, io.BytesIO(data))
        digest = hashlib.sha256(archive.read_bytes()).hexdigest()
        (self.assets / (self.bundle_name + ".sha256")).write_text(f"{digest}  {self.bundle_name}\n")
        result = self.run_apply()
        update_status = json.loads((self.web / "update_status.js").read_text().split("=", 1)[1].strip().rstrip(";"))
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr + "\n" + update_status.get("message", ""))
        status = json.loads((self.web / "update_status.js").read_text().split("=", 1)[1].strip().rstrip(";"))
        self.assertEqual(status["status"], "SUCCESS")
        self.assertEqual(status["request_id"], "fixture")
        self.assertFalse((self.root / "update.lock").exists())


if __name__ == "__main__":
    unittest.main()
