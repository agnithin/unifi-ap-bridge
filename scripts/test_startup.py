#!/usr/bin/env python3
"""Exercise normal startup entry points with synthetic configs and fake processes."""
import os
import io
import hashlib
from pathlib import Path
import subprocess
import tarfile
import tempfile
import time
import unittest
ROOT=Path(__file__).resolve().parent.parent
class StartupTests(unittest.TestCase):
    def setUp(self):
        self.tmp=tempfile.TemporaryDirectory(dir="/tmp");self.addCleanup(self.tmp.cleanup);self.root=Path(self.tmp.name)
        (self.root/"tmp").mkdir()
        self.install=self.root/"addon";(self.install/"config").mkdir(parents=True);(self.install/"bin").mkdir()
        self.bin=self.root/"bin";self.bin.mkdir();self.launches=self.root/"launches"
        self.env=dict(os.environ,PATH=str(self.bin)+os.pathsep+os.environ["PATH"],LAUNCHES=str(self.launches),STATUS_FILE=str(self.root/"tmp/var/wwwext/unifi-bridge/status.js"))
        for name,body in {"killall":"exit 0","cru":"exit 0","uname":"echo armv7l","pidof":"exit 0","sync":"exit 0"}.items():
            p=self.bin/name;p.write_text("#!/bin/sh\n"+body+"\n");p.chmod(0o755)
        daemon=self.install/"bin/unifi-bridge";daemon.write_text('#!/usr/bin/env python3\nimport json,os,sys,time\nfrom pathlib import Path\ncfg=json.loads(Path(sys.argv[sys.argv.index(\'-config\')+1]).read_text())\nif \'-is-enabled\' in sys.argv:sys.exit(0 if cfg.get(\'enabled\',True) else 3)\nif \'-once\' in sys.argv and cfg.get(\'enabled\') is False:\n    p=Path(os.environ[\'STATUS_FILE\']);p.parent.mkdir(parents=True,exist_ok=True);p.write_text(\'window.unifiBridgeStatus = {"running":false,"enabled":false,"version":"1.2.4","commit":"'+'a'*40+'","status_updated_at":\'+str(int(time.time()))+\'};\');sys.exit(0)\nwith open(os.environ[\'LAUNCHES\'],\'a\') as f:f.write(\'launched\\n\')\n');daemon.chmod(0o755)
        source=(ROOT/"scripts/start.sh").read_text().replace('/tmp/',str(self.root/"tmp")+'/').replace('/jffs/addons/unifi-bridge',str(self.install))
        self.script=self.root/"start.sh";self.script.write_text(source)
    def run_start(self):
        lock=self.root/"tmp/unifi-bridge-start.lock"
        self.assertFalse(lock.exists(),"lock already exists before startup")
        result=subprocess.run(["sh",str(self.script)],env=self.env,capture_output=True,text=True,timeout=5)
        self.assertEqual(result.returncode,0,result.stdout+result.stderr)
        self.assertFalse(list(self.root.glob("**/unifi-bridge-start.lock")),"a nested start lock leaked")
        self.assertFalse(lock.exists(),"start lock leaked: "+str(list(lock.iterdir()) if lock.exists() else "missing"))
        self.assertFalse((self.root/"tmp/unifi-bridge-mutation.lock").exists(),"mutation lock leaked")
    def test_start_refuses_busy_mutation_lock_without_stealing(self):
        lock=self.root/"tmp/unifi-bridge-mutation.lock";lock.mkdir();(lock/"pid").write_text("424242\n")
        self.env["UNIFI_BRIDGE_MUTATION_WAIT_SECONDS"]="0"
        result=subprocess.run(["sh",str(self.script)],env=self.env,capture_output=True,text=True,timeout=3)
        self.assertNotEqual(result.returncode,0)
        self.assertIn("Mutation lock busy",result.stderr)
        self.assertEqual((lock/"pid").read_text(),"424242\n")
        self.assertFalse(self.launches.exists())
    def test_busy_start_lock_releases_new_mutation_lock(self):
        lock=self.root/"tmp/unifi-bridge-start.lock";lock.mkdir();(lock/"pid").write_text("424242\n")
        result=subprocess.run(["sh",str(self.script)],env=self.env,capture_output=True,text=True,timeout=3)
        self.assertNotEqual(result.returncode,0)
        self.assertIn("Another bridge start",result.stderr)
        self.assertEqual((lock/"pid").read_text(),"424242\n")
        self.assertFalse((self.root/"tmp/unifi-bridge-mutation.lock").exists(),"mutation lock leaked after busy start lock")

    def test_disabled_survives_all_start_invocations(self):
        for raw in ['{"enabled":false}', '{\n "enabled" : false\n}']:
            (self.install/"config/config.json").write_text(raw)
            self.run_start();self.run_start()
            self.assertFalse(self.launches.exists())
            status=self.root/"tmp/var/wwwext/unifi-bridge/status.js"
            self.assertIn('"running":false',status.read_text())
    def test_enabled_can_start_after_disable(self):
        conf=self.install/"config/config.json";conf.write_text('{"enabled":false}');self.run_start()
        conf.write_text('{"enabled":true,"extra":{"enabled":false}}');self.run_start()
        for _ in range(50):
            if self.launches.exists():break
            time.sleep(.01)
        self.assertEqual(self.launches.read_text().splitlines(),['launched'])
    def test_accepted_journal_cleanup_preserves_candidate_generation(self):
        txn=self.install/".update-transaction";txn.mkdir()
        (txn/"manifest").write_text("present version.txt\n")
        (txn/"active").write_text("ACCEPTED\n")
        (self.install/"version.txt").write_text("1.2.5\n")
        (self.install/"config/config.json").write_text('{"enabled":false}')
        self.run_start()
        self.assertFalse(txn.exists())
        self.assertEqual((self.install/"version.txt").read_text(),"1.2.5\n")
        self.assertFalse(list(self.install.glob(".update-complete-*")))

    def test_interrupted_activation_restores_previous_disabled_generation(self):
        txn=self.install/".update-transaction";snapshot=txn/"snapshot";(snapshot/"webui").mkdir(parents=True)
        (self.install/"webui").mkdir(parents=True)
        (self.install/"config/config.json").write_text('{"enabled":false,"unknown":"preserve"}')
        prior_binary=self.root/"prior-binary"
        prior_binary.write_text(
            '#!/bin/sh\ncase "$*" in *-version*) echo "unifi-bridge 1.2.4 (commit: ' + 'a'*40 + ', built: fixture)";; '
            '*-is-enabled*) exit 3;; *-once*) cat > "$STATUS_FILE" <<EOF\n'
            'window.unifiBridgeStatus = {"running":false,"enabled":false,"version":"1.2.4","commit":"' + 'a'*40 + '","status_updated_at":$(date +%s)};\nEOF\n;; esac\n'
        );prior_binary.chmod(0o755)
        (snapshot/"start.sh").write_text("#!/bin/sh\necho prior\n")
        (snapshot/"version.txt").write_text("1.2.4\n")
        (snapshot/"webui/unifi-bridge.asp").write_text("prior-ui")
        bundle_files={"apply_config.sh":b"#!/bin/sh\n", "bundle.json":('{"version":"1.2.4","commit":"'+'a'*40+'","architecture":"armv7"}\n').encode(),
            "install.sh":b"#!/bin/sh\n", "start.sh":(snapshot/"start.sh").read_bytes(), "sync_nodes.sh":b"#!/bin/sh\n",
            "unifi-bridge-armv7":prior_binary.read_bytes(), "unifi-bridge.asp":b"prior-ui", "unifi-bridge.sh":b"#!/bin/sh\n",
            "update.sh":b"#!/bin/sh\n", "version.txt":b"1.2.4\n", "watchdog.sh":b"#!/bin/sh\n"}
        with tarfile.open(snapshot/"unifi-bridge.tar.gz","w:gz",format=tarfile.USTAR_FORMAT) as tar:
            for name,data in bundle_files.items():
                info=tarfile.TarInfo(name);info.mode=0o755 if name.endswith(".sh") or name.startswith("unifi-bridge-arm") else 0o644;info.size=len(data);tar.addfile(info,io.BytesIO(data))
        prior_digest=hashlib.sha256((snapshot/"unifi-bridge.tar.gz").read_bytes()).hexdigest()
        (snapshot/"unifi-bridge.tar.gz.sha256").write_text(prior_digest+"  unifi-bridge.tar.gz\n")
        names=["unifi-bridge.tar.gz","unifi-bridge.tar.gz.sha256","start.sh","apply_config.sh","sync_nodes.sh","unifi-bridge.sh","update.sh","watchdog.sh","version.txt","webui/unifi-bridge.asp"]
        (txn/"manifest").write_text("".join("present "+name+"\n" if (snapshot/name).is_file() else "absent "+name+"\n" for name in names))
        (txn/"active").write_text("ACTIVATING\n")
        for name in names:
            dest=self.install/name
            if name=="unifi-bridge.tar.gz":
                with tarfile.open(dest,"w:gz") as tar:tar.add(prior_binary,arcname="unifi-bridge-armv7")
            elif name=="webui/unifi-bridge.asp":dest.write_text("candidate-ui")
            elif name=="version.txt":dest.write_text("9.9.9\n")
            elif name=="start.sh":dest.write_text("#!/bin/sh\necho candidate\n")
            elif name!="unifi-bridge.tar.gz":dest.parent.mkdir(parents=True,exist_ok=True);dest.write_text("candidate\n")
        self.run_start()
        self.assertFalse(txn.exists())
        self.assertEqual((self.install/"version.txt").read_text(),"1.2.4\n")
        self.assertEqual((self.install/"webui/unifi-bridge.asp").read_text(),"prior-ui")
        self.assertIn("prior",(self.install/"start.sh").read_text())
        self.assertEqual((self.install/"config/config.json").read_text(),'{"enabled":false,"unknown":"preserve"}')
        self.run_start() # recovery and disabled publication are idempotent
        self.assertFalse(txn.exists())
    def test_watchdog_defers_during_start_or_unaccepted_activation(self):
        (self.install/"config/config.json").write_text('{"enabled":true}')
        marker=self.root/"watchdog-started"
        start=self.install/"start.sh";start.write_text("#!/bin/sh\ntouch '"+str(marker)+"'\n");start.chmod(0o755)
        source=(ROOT/"scripts/watchdog.sh").read_text().replace('/tmp/','__TMPROOT__/').replace('/jffs/addons/unifi-bridge',str(self.install)).replace('__TMPROOT__/',str(self.root/"tmp")+'/')
        script=self.root/"watchdog.sh";script.write_text(source)
        lock=self.root/"tmp/unifi-bridge-start.lock";lock.mkdir()
        subprocess.run(["sh",str(script)],env=self.env,check=True,timeout=3)
        self.assertFalse(marker.exists())
        lock.rmdir()
        txn=self.install/".update-transaction";txn.mkdir();(txn/"active").write_text("ACTIVATING\n")
        subprocess.run(["sh",str(script)],env=self.env,check=True,timeout=3)
        self.assertFalse(marker.exists())
if __name__=="__main__":unittest.main()
