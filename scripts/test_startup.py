#!/usr/bin/env python3
"""Exercise normal startup entry points with synthetic configs and fake processes."""
import os
from pathlib import Path
import subprocess
import tempfile
import time
import unittest
ROOT=Path(__file__).resolve().parent.parent
class StartupTests(unittest.TestCase):
    def setUp(self):
        self.tmp=tempfile.TemporaryDirectory();self.addCleanup(self.tmp.cleanup);self.root=Path(self.tmp.name)
        self.install=self.root/"addon";(self.install/"config").mkdir(parents=True);(self.install/"bin").mkdir()
        self.bin=self.root/"bin";self.bin.mkdir();self.launches=self.root/"launches"
        self.env=dict(os.environ,PATH=str(self.bin)+os.pathsep+os.environ["PATH"],LAUNCHES=str(self.launches),STATUS_FILE=str(self.root/"tmp/var/wwwext/unifi-bridge/status.js"))
        for name,body in {"killall":"exit 0","cru":"exit 0"}.items():
            p=self.bin/name;p.write_text("#!/bin/sh\n"+body+"\n");p.chmod(0o755)
        daemon=self.install/"bin/unifi-bridge";daemon.write_text('#!/usr/bin/env python3\nimport json,os,sys\nfrom pathlib import Path\ncfg=json.loads(Path(sys.argv[sys.argv.index(\'-config\')+1]).read_text())\nif \'-is-enabled\' in sys.argv:sys.exit(0 if cfg.get(\'enabled\',True) else 3)\nif \'-once\' in sys.argv and cfg.get(\'enabled\') is False:\n    p=Path(os.environ[\'STATUS_FILE\']);p.parent.mkdir(parents=True,exist_ok=True);p.write_text(\'window.unifiBridgeStatus = {"running":false};\');sys.exit(0)\nwith open(os.environ[\'LAUNCHES\'],\'a\') as f:f.write(\'launched\\n\')\n');daemon.chmod(0o755)
        source=(ROOT/"scripts/start.sh").read_text().replace('/jffs/addons/unifi-bridge',str(self.install)).replace('/tmp/',str(self.root/"tmp")+'/')
        self.script=self.root/"start.sh";self.script.write_text(source)
    def run_start(self):
        subprocess.run(["sh",str(self.script)],env=self.env,check=True,capture_output=True,timeout=5)
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
if __name__=="__main__":unittest.main()
