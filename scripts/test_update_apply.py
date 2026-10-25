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
        self.write(self.bin / "sync", "#!/bin/sh\nexit 0\n")
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
        source = source.replace('/tmp/', '__TMPROOT__/')
        source = source.replace('/jffs/addons/unifi-bridge', str(self.install))
        source = source.replace('__TMPROOT__/unifi-bridge-start.lock', str(self.root / 'start.lock'))
        source = source.replace('__TMPROOT__/unifi-bridge-mutation.lock', str(self.root / 'mutation.lock'))
        source = source.replace('__TMPROOT__/', str(self.root / 'tmp') + '/')
        return source

    def run_apply(self, env=None, args=None):
        return subprocess.run(
            ["sh", str(self.script), *(args or ["apply", "dev", "fixture"])],
            env=env or self.env,
            capture_output=True,
            text=True,
            timeout=40,
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

    def make_main_compatibility_generation(self):
        # Main's observed bootstrap layout: prior daemon/scripts and prior
        # bundle metadata, with the candidate updater installed in both places.
        # Its standalone installer also predates the archive and is unmanaged.
        self.write(self.install / "install.sh", "#!/bin/sh\necho legacy-installer\n")
        self.write(self.install / "start.sh", self.compatibility_start_source("start_be106.sh"))
        self.write(self.install / "update.sh", (ROOT / "scripts/update.sh").read_text())
        self.make_prior_bundle()

    def make_node_compatibility_generation(self):
        # Node's 6224430 startup can recover journals, but its allowlist does
        # not include install.sh. Preserve that exact old behavior as a fixture.
        self.write(self.install / "install.sh", "#!/bin/sh\necho older-node-installer\n")
        self.write(self.install / "start.sh", self.compatibility_start_source("start_6224430.sh"))
        self.write(self.install / "update.sh", (ROOT / "scripts/update.sh").read_text())
        self.make_prior_bundle()

    def compatibility_start_source(self, name):
        source = (ROOT / "scripts/fixtures" / name).read_text()
        source = source.replace('/tmp/', '__TMPROOT__/')
        source = source.replace('/jffs/addons/unifi-bridge', str(self.install))
        source = source.replace('__TMPROOT__/unifi-bridge-start.lock', str(self.root / 'start.lock'))
        source = source.replace('__TMPROOT__/unifi-bridge-mutation.lock', str(self.root / 'mutation.lock'))
        source = source.replace('__TMPROOT__/', str(self.root / 'tmp') + '/')
        return source

    def generation_snapshot(self):
        paths = [
            "unifi-bridge.tar.gz", "unifi-bridge.tar.gz.sha256", "install.sh", "start.sh",
            "apply_config.sh", "sync_nodes.sh", "unifi-bridge.sh", "update.sh",
            "watchdog.sh", "version.txt", "webui/unifi-bridge.asp",
            "config/config.json", "config/state.json", ".ssh/cluster_key",
        ]
        result = {}
        for rel in paths:
            path = self.install / rel
            result[rel] = None if not path.exists() else (path.read_bytes(), path.stat().st_mode & 0o777)
        return result

    def inject_missing_staged_asset(self, *, whole_stage):
        self.write(self.bin / "cp",
            "#!/usr/bin/env python3\nimport sys,subprocess,shutil\nfrom pathlib import Path\n"
            "source=next((Path(arg) for arg in sys.argv[1:] if '/staged/update.sh' in arg),None)\n"
            "if source is not None:\n"
            + (" shutil.rmtree(source.parent)\n" if whole_stage else " source.unlink()\n")
            + "sys.exit(subprocess.call(['/bin/cp']+sys.argv[1:]))\n")

    def remove_staged_update_during_extract(self):
        real_tar = shutil.which("tar", path=os.pathsep.join(self.env["PATH"].split(os.pathsep)[1:]))
        self.write(self.bin / "tar",
            "#!/usr/bin/env python3\nimport sys,subprocess\nfrom pathlib import Path\n"
            f"args=sys.argv[1:]; rc=subprocess.call([{real_tar!r}]+args)\n"
            "if rc == 0 and '-xzf' in args and '-C' in args:\n"
            " target=Path(args[args.index('-C')+1])\n"
            " if target.name == 'staged': (target/'update.sh').unlink()\n"
            "sys.exit(rc)\n")

    def test_verification_helpers_reject_failed_or_invalid_tool_output(self):
        source = (ROOT / "scripts/update.sh").read_text()
        start = source.index("file_sha256() {")
        end = source.index("verify_installed_generation() {")
        helpers = source[start:end]
        sample = self.root / "sample"
        sample.write_text("fixture")
        cases = (
            ("openssl", "file_sha256", "", 1),
            ("openssl", "file_sha256", "not-a-digest", 0),
            ("openssl", "file_sha256", "SHA2-256(file)= " + "a" * 64, 1),
            ("ls", "file_mode", "", 1),
            ("ls", "file_mode", "not-a-mode", 0),
            ("ls", "file_mode", "-rwxr-xr-x 1 user group 1 Jan 1 00:00 sample", 1),
        )
        for command, helper, output, exit_code in cases:
            with self.subTest(command=command, output=output, exit_code=exit_code):
                wrapper = self.bin / command
                wrapper.write_text(
                    "#!/bin/sh\nprintf '%s\\n' " + repr(output) + "\nexit " + str(exit_code) + "\n"
                )
                wrapper.chmod(0o755)
                result = subprocess.run(
                    ["sh", "-c", helpers + "\n" + helper + ' "$1"', "test", str(sample)],
                    env=dict(self.env, PATH=str(self.bin) + os.pathsep + os.environ["PATH"]),
                    capture_output=True, text=True, timeout=3,
                )
                self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)

    def assert_mixed_generation_recovery(self, result, expected_snapshot, request_id):
        output = result.stdout + result.stderr
        self.assertNotEqual(result.returncode, 0, output)
        self.assertIn("staged/update.sh", output)
        self.assertTrue("can't stat" in output or "No such file" in output, output)
        status = json.loads((self.web / "update_status.js").read_text().split("=", 1)[1].strip().rstrip(";"))
        self.assertEqual(status["status"], "ERROR")
        self.assertIn("restored and verified", status["message"])
        operation = json.loads((self.web / "cluster_operation.js").read_text().split("=", 1)[1].strip().rstrip(";"))
        self.assertEqual(operation["operation_id"], request_id)
        self.assertEqual(operation["status"], "FAILED")
        current_snapshot = self.generation_snapshot()
        changed = [rel for rel in expected_snapshot if current_snapshot[rel] != expected_snapshot[rel]]
        self.assertEqual(current_snapshot, expected_snapshot, f"restored generation differs at: {changed}")
        self.assertEqual((self.install / "config/config.json").read_text(), '{"branch":"dev"}')
        self.assertEqual((self.install / "config/state.json").read_text(), '{"adopted":true,"identity":"preserve"}')
        self.assertEqual((self.install / ".ssh/cluster_key").read_text(), "private identity fixture")
        self.assertFalse((self.install / ".update-transaction").exists())
        self.assertFalse((self.root / "update.lock").exists())
        self.assertFalse((self.root / "mutation.lock").exists())
        self.assertFalse((self.root / "start.lock").exists())

    def test_apply_exact_missing_staged_update_script_rolls_back_mixed_prior(self):
        self.make_main_compatibility_generation()
        before = self.generation_snapshot()
        self.inject_missing_staged_asset(whole_stage=False)
        request_id = "777126"
        result = self.run_apply(args=["apply-exact", "dev", COMMIT, request_id])
        self.assert_mixed_generation_recovery(result, before, request_id)

    def test_legacy_main_entry_failure_before_recovery_entry_leaves_prior_unchanged(self):
        self.make_main_compatibility_generation()
        before = self.generation_snapshot()
        self.write(self.bin / "cp",
            "#!/usr/bin/env python3\nimport sys\n"
            "if any('/staged/start.sh' in x for x in sys.argv): sys.exit(1)\n"
            "import subprocess; sys.exit(subprocess.call(['/bin/cp']+sys.argv[1:]))\n")
        result = self.run_apply(args=["apply-exact", "dev", COMMIT, "777129"])
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(self.generation_snapshot(), before)
        self.assertFalse((self.install / ".update-transaction").exists())
        self.assertTrue(self.running.exists())
        self.assertFalse((self.root / "mutation.lock").exists())
        self.assertFalse((self.root / "start.lock").exists())

    def test_manifest_promotion_failure_rolls_back_before_installer_replacement(self):
        self.make_main_compatibility_generation()
        before = self.generation_snapshot()
        self.write(self.bin / "mv",
            "#!/usr/bin/env python3\nimport sys,subprocess\n"
            "if any('manifest.promote' in x for x in sys.argv): sys.exit(1)\n"
            "sys.exit(subprocess.call(['/bin/mv']+sys.argv[1:]))\n")
        result = self.run_apply(args=["apply-exact", "dev", COMMIT, "777130"])
        output = result.stdout + result.stderr
        self.assertNotEqual(result.returncode, 0, output)
        status = json.loads((self.web / "update_status.js").read_text().split("=", 1)[1].strip().rstrip(";"))
        self.assertIn("Full recovery manifest promotion failed", status["message"])
        self.assertIn("restored and verified", status["message"])
        self.assertEqual(self.generation_snapshot(), before)
        self.assertFalse((self.install / ".update-transaction").exists())
        self.assertFalse((self.root / "update.lock").exists())
        self.assertFalse((self.root / "mutation.lock").exists())
        self.assertFalse((self.root / "start.lock").exists())

    def test_corrupt_recovery_entry_copy_restores_prior_entry_without_running_candidate(self):
        self.make_main_compatibility_generation()
        before = self.generation_snapshot()
        self.write(self.bin / "cp",
            "#!/usr/bin/env python3\nimport sys,subprocess\n"
            "result=subprocess.call(['/bin/cp']+sys.argv[1:])\n"
            "if result == 0 and any('start.sh.update.' in x for x in sys.argv):\n"
            "  open(sys.argv[-1], 'ab').write(b'\\n# injected corrupt copy\\n')\n"
            "sys.exit(result)\n")
        result = self.run_apply(args=["apply-exact", "dev", COMMIT, "777131"])
        output = result.stdout + result.stderr
        self.assertNotEqual(result.returncode, 0, output)
        status = json.loads((self.web / "update_status.js").read_text().split("=", 1)[1].strip().rstrip(";"))
        self.assertIn("Recovery startup content verification failed", status["message"])
        self.assertIn("previous entry, assets and runtime restored and verified", status["message"])
        self.assertEqual(self.generation_snapshot(), before)
        self.assertTrue(self.running.exists())
        self.assertFalse((self.install / ".update-transaction").exists())
        self.assertFalse((self.root / "mutation.lock").exists())
        self.assertFalse((self.root / "start.lock").exists())

    def test_interruption_after_entry_install_recovers_generated_legacy_journal(self):
        self.make_main_compatibility_generation()
        before = self.generation_snapshot()
        self.write(self.bin / "cp",
            "#!/usr/bin/env python3\nimport os,sys,subprocess\n"
            "result=subprocess.call(['/bin/cp']+sys.argv[1:])\n"
            "if result == 0 and any('manifest.promote' in x for x in sys.argv):\n"
            "  os.kill(os.getppid(), 9)\n"
            "sys.exit(result)\n")
        interrupted = self.run_apply(args=["apply-exact", "dev", COMMIT, "777132"])
        self.assertNotEqual(interrupted.returncode, 0)
        txn = self.install / ".update-transaction"
        self.assertTrue((txn / "active").exists())
        self.assertTrue((txn / "manifest.full").exists())
        self.assertNotIn("install.sh", (txn / "manifest").read_text())
        self.assertIn("install.sh", (txn / "manifest.full").read_text())
        self.assertNotEqual((self.install / "start.sh").read_bytes(), before["start.sh"][0])

        # SIGKILL models power loss: the lock owner has exited but cleanup
        # traps could not run. Recovery tooling removes only this fixture's
        # stale locks after subprocess completion proves the owner is gone.
        for lock in (self.root / "mutation.lock", self.root / "update.lock", self.root / "start.lock"):
            if lock.is_dir():
                shutil.rmtree(lock)
        recovery_env = dict(self.env, STATUS_FILE=str(self.root / "tmp/var/wwwext/unifi-bridge/status.js"))
        recovered = subprocess.run(["sh", str(self.install / "start.sh")],
            env=recovery_env, capture_output=True, text=True, timeout=8)
        self.assertEqual(recovered.returncode, 0, recovered.stdout + recovered.stderr)
        self.assertEqual(self.generation_snapshot(), before)
        self.assertTrue(self.running.exists())
        self.assertFalse(txn.exists(), "normal startup verifies prior state and completes recovery cleanup")

    def make_inert_main_journal(self):
        self.make_main_compatibility_generation()
        before = self.generation_snapshot()
        self.write(self.bin / "cp",
            "#!/usr/bin/env python3\nimport os,sys,subprocess\nfrom pathlib import Path\n"
            "marker=Path(os.environ['CP_KILL_MARKER'])\n"
            "result=subprocess.call(['/bin/cp']+sys.argv[1:])\n"
            "if result == 0 and any('start.sh.update.' in x for x in sys.argv) and not marker.exists():\n"
            "  marker.touch()\n  os.kill(os.getppid(), 9)\n"
            "sys.exit(result)\n")
        env = dict(self.env, CP_KILL_MARKER=str(self.root / "cp-killed"))
        interrupted = self.run_apply(env=env, args=["apply-exact", "dev", COMMIT, "777133"])
        self.assertNotEqual(interrupted.returncode, 0)
        txn = self.install / ".update-transaction"
        self.assertTrue((txn / "active").exists())
        self.assertTrue((txn / "manifest.full").exists())
        self.assertEqual(self.generation_snapshot(), before,
            "all prior managed assets remain unchanged before candidate entry rename")
        for lock in (self.root / "mutation.lock", self.root / "update.lock", self.root / "start.lock"):
            if lock.is_dir():
                shutil.rmtree(lock)
        return env, txn, before

    def test_inert_main_journal_is_retired_only_after_exact_snapshot_match(self):
        env, txn, before = self.make_inert_main_journal()
        retry = self.run_apply(env=env, args=["apply-exact", "dev", COMMIT, "777134"])
        output = retry.stdout + retry.stderr
        self.assertNotEqual(retry.returncode, 0, output)  # candidate startup fixture intentionally fails
        status = json.loads((self.web / "update_status.js").read_text().split("=", 1)[1].strip().rstrip(";"))
        self.assertNotIn("snapshot did not prove", status["message"])
        self.assertIn("previous addon generation restored and verified", status["message"])
        self.assertFalse(txn.exists())
        self.assertEqual(self.generation_snapshot(), before)

    def test_inert_main_journal_with_asset_mismatch_is_retained(self):
        env, txn, _before = self.make_inert_main_journal()
        self.write(self.install / "watchdog.sh", "#!/bin/sh\necho altered\n")
        mismatch = self.generation_snapshot()
        retry = self.run_apply(env=env, args=["apply-exact", "dev", COMMIT, "777135"])
        output = retry.stdout + retry.stderr
        self.assertNotEqual(retry.returncode, 0, output)
        status = json.loads((self.web / "update_status.js").read_text().split("=", 1)[1].strip().rstrip(";"))
        self.assertIn("snapshot did not prove a complete unchanged generation", status["message"])
        self.assertTrue(txn.exists())
        self.assertEqual(self.generation_snapshot(), mismatch)

    def test_inert_main_journal_rejects_unterminated_manifest_and_final_asset_change(self):
        env, txn, _before = self.make_inert_main_journal()
        manifest = txn / "manifest.full"
        manifest.write_bytes(manifest.read_bytes().rstrip(b"\n"))
        self.write(self.install / "webui/unifi-bridge.asp", "altered final manifest asset")
        mismatch = self.generation_snapshot()
        retry = self.run_apply(env=env, args=["apply-exact", "dev", COMMIT, "777136"])
        output = retry.stdout + retry.stderr
        self.assertNotEqual(retry.returncode, 0, output)
        status = json.loads((self.web / "update_status.js").read_text().split("=", 1)[1].strip().rstrip(";"))
        self.assertIn("snapshot did not prove a complete unchanged generation", status["message"])
        self.assertTrue(txn.exists())
        self.assertEqual(self.generation_snapshot(), mismatch)

    def test_622_legacy_entry_accepts_compat_manifest_and_rejects_full_manifest(self):
        self.make_node_compatibility_generation()
        txn = self.install / ".update-transaction"
        (txn / "snapshot").mkdir(parents=True)
        full, legacy = [], []
        managed = [
            "unifi-bridge.tar.gz", "unifi-bridge.tar.gz.sha256", "install.sh", "start.sh",
            "apply_config.sh", "sync_nodes.sh", "unifi-bridge.sh", "update.sh",
            "watchdog.sh", "version.txt", "webui/unifi-bridge.asp",
        ]
        snapshot = self.generation_snapshot()
        for rel in managed:
            value = snapshot[rel]
            if value is None:
                full.append(f"absent {rel}\n")
                if rel != "install.sh":
                    legacy.append(f"absent {rel}\n")
            else:
                data, mode = value
                target = txn / "snapshot" / rel
                target.parent.mkdir(parents=True, exist_ok=True)
                target.write_bytes(data); target.chmod(mode)
                full.append(f"present {rel}\n")
                if rel != "install.sh":
                    legacy.append(f"present {rel}\n")
        (txn / "manifest.full").write_text("".join(full))
        (txn / "manifest").write_text("".join(legacy))
        (txn / "active").write_text("ACTIVATING\n")
        legacy_entry = self.root / "legacy-start.sh"
        legacy_entry.write_text(self.compatibility_start_source("start_6224430.sh"))
        legacy_entry.chmod(0o755)
        result = subprocess.run(["sh", str(legacy_entry), "--recover-only"], env=self.env,
            capture_output=True, text=True, timeout=8)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertTrue(txn.exists(), "recover-only must leave transaction cleanup to the updater")
        self.assertIn("install.sh", "".join(full))
        self.assertNotIn("install.sh", "".join(legacy))

        (txn / "manifest").write_text("".join(full))
        rejected = subprocess.run(["sh", str(legacy_entry), "--recover-only"], env=self.env,
            capture_output=True, text=True, timeout=8)
        self.assertNotEqual(rejected.returncode, 0, rejected.stdout + rejected.stderr)
        self.assertTrue(txn.exists(), "unsupported full manifest must retain recovery evidence")

    def test_apply_exact_removed_stage_tree_rolls_back_mixed_prior(self):
        self.make_main_compatibility_generation()
        before = self.generation_snapshot()
        self.inject_missing_staged_asset(whole_stage=True)
        request_id = "777127"
        result = self.run_apply(args=["apply-exact", "dev", COMMIT, request_id])
        self.assert_mixed_generation_recovery(result, before, request_id)

    def test_apply_exact_missing_staged_asset_fails_before_transaction(self):
        self.make_main_compatibility_generation()
        before = self.generation_snapshot()
        self.remove_staged_update_during_extract()
        request_id = "777128"
        result = self.run_apply(args=["apply-exact", "dev", COMMIT, request_id])
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        status = json.loads((self.web / "update_status.js").read_text().split("=", 1)[1].strip().rstrip(";"))
        self.assertEqual(status["status"], "ERROR")
        self.assertIn("Extracted bundle lost a required staged asset", status["message"])
        self.assertEqual(self.generation_snapshot(), before)
        self.assertFalse((self.install / ".update-transaction").exists())
        self.assertFalse((self.root / "mutation.lock").exists())
        self.assertFalse((self.root / "start.lock").exists())
        operation = json.loads((self.web / "cluster_operation.js").read_text().split("=", 1)[1].strip().rstrip(";"))
        self.assertEqual(operation["operation_id"], request_id)
        self.assertEqual(operation["status"], "FAILED")

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
        self.make_main_compatibility_generation()
        self.write(self.bin / "nohup",
                   '#!/bin/sh\ntouch "$RUNNING"\ncat > "$STATUS_FILE" <<EOF\n'
                   'window.unifiBridgeStatus = {"running": true, "enabled": true, "version": "1.2.5", "commit": "' + COMMIT + '", "pid": 1234, "status_updated_at": ' + '$(date +%s)' + ', "last_successful_inform": "2026-10-25 17:20:00", "last_status": "Active (GCM)"};\nEOF\n')
        result=self.run_apply()
        self.assertEqual(result.returncode,0,result.stdout+result.stderr+(self.web/"update_status.js").read_text())
        status=json.loads((self.web/"update_status.js").read_text().split("=",1)[1].strip().rstrip(";"))
        self.assertEqual(status["status"],"SUCCESS")
        self.assertEqual((self.install/"version.txt").read_text(),"1.2.5\n")
        self.assertFalse((self.install/".update-transaction").exists())
        self.assertEqual((self.install/"config/config.json").read_text(),'{"branch":"dev"}')
        self.assertEqual((self.install/"config/state.json").read_text(),'{"adopted":true,"identity":"preserve"}')
        self.assertEqual((self.install/".ssh/cluster_key").read_text(),"private identity fixture")
        installed_manifest = (self.install / "unifi-bridge.tar.gz.sha256").read_text()
        self.assertRegex(installed_manifest, r"^[0-9a-f]{64}  unifi-bridge\.tar\.gz\n$")
        self.assertEqual((self.install / "unifi-bridge.tar.gz").read_bytes(), (self.assets / self.bundle_name).read_bytes())
        with tarfile.open(self.install / "unifi-bridge.tar.gz", "r:gz") as archive:
            for member, installed in (
                ("install.sh", "install.sh"), ("start.sh", "start.sh"),
                ("apply_config.sh", "apply_config.sh"), ("sync_nodes.sh", "sync_nodes.sh"),
                ("unifi-bridge.sh", "unifi-bridge.sh"), ("update.sh", "update.sh"),
                ("watchdog.sh", "watchdog.sh"), ("version.txt", "version.txt"),
                ("unifi-bridge.asp", "webui/unifi-bridge.asp"),
            ):
                self.assertEqual((self.install / installed).read_bytes(), archive.extractfile(member).read(), member)

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
