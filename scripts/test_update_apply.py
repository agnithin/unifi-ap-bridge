#!/usr/bin/env python3
"""Verify addon update failures cannot masquerade as successful installs."""
import hashlib
import io
import json
import os
from pathlib import Path
import subprocess
import tarfile
import tempfile
import unittest
ROOT=Path(__file__).resolve().parent.parent
class ApplyFailureTests(unittest.TestCase):
    def setUp(self):
        self.tmp=tempfile.TemporaryDirectory();self.addCleanup(self.tmp.cleanup);self.root=Path(self.tmp.name)
        self.install=self.root/"addon";(self.install/"config").mkdir(parents=True)
        (self.install/"config/config.json").write_text('{"branch":"dev"}')
        (self.install/"version.txt").write_text('1.2.4\n')
        self.web=self.root/"www/unifi-bridge";self.web.mkdir(parents=True)
        self.bin=self.root/"bin";self.bin.mkdir();self.assets=self.root/"assets";self.assets.mkdir()
        self.env=dict(os.environ,PATH=str(self.bin)+os.pathsep+os.environ["PATH"],ASSETS=str(self.assets))
        self.write(self.bin/"uname",'#!/bin/sh\necho armv7l\n')
        self.write(self.bin/"sleep",'#!/bin/sh\nexit 0\n')
        self.write(self.bin/"curl","#!/usr/bin/env python3\nimport os,sys,shutil\nfrom pathlib import Path\na=sys.argv[1:];url=next((v for v in a if v.startswith('https://')), '');name=url.split('?')[0].rsplit('/',1)[-1]\nsource=Path(os.environ['ASSETS'])/name\nif not source.exists():sys.exit(22)\nshutil.copyfile(source,a[a.index('-o')+1])\n")
        for name in ['start.sh','apply_config.sh','sync_nodes.sh','unifi-bridge.sh','update.sh','watchdog.sh']:
            self.write(self.assets/name,'#!/bin/sh\nexit 42\n')
        (self.assets/'unifi-bridge.asp').write_text('candidate')
        (self.assets/'version.txt').write_text('1.2.5\n')
        body=b'#!/bin/sh\necho "unifi-bridge 1.2.5 (commit: test, built: test)"\n'
        archive=self.assets/'unifi-bridge-armv7.tar.gz'
        with tarfile.open(archive,'w:gz') as t:
            info=tarfile.TarInfo('unifi-bridge-armv7');info.mode=0o755;info.size=len(body);t.addfile(info,io.BytesIO(body))
        (self.assets/'sha256sums.txt').write_text(hashlib.sha256(archive.read_bytes()).hexdigest()+'  unifi-bridge-armv7.tar.gz\n')
        source=(ROOT/'scripts/update.sh').read_text().replace('INSTALL_DIR="/jffs/addons/$ADDON_NAME"','INSTALL_DIR="'+str(self.install)+'"')
        source=source.replace('/tmp/var/wwwext',str(self.root/'www')).replace('/jffs/scripts',str(self.root/'links'))
        (self.root/'links').mkdir();self.script=self.root/'update.sh';self.script.write_text(source)
    def write(self,path,text):path.write_text(text);path.chmod(0o755)
    def run_failure(self):
        p=subprocess.run(['sh',str(self.script),'apply','dev','fixture'],env=self.env,capture_output=True,text=True,timeout=8)
        self.assertNotEqual(p.returncode,0,p.stdout)
        raw=(self.web/'update_status.js').read_text();status=json.loads(raw.split('=',1)[1].strip().rstrip(';'))
        self.assertEqual(status['status'],'ERROR')
        return status
    def test_missing_binary_keeps_existing_version(self):
        (self.assets/'unifi-bridge-armv7.tar.gz').unlink();self.run_failure()
        self.assertEqual((self.install/'version.txt').read_text(),'1.2.4\n')
    def test_bad_checksum_keeps_existing_version(self):
        (self.assets/'sha256sums.txt').write_text('0'*64+'  unifi-bridge-armv7.tar.gz\n');self.run_failure()
        self.assertEqual((self.install/'version.txt').read_text(),'1.2.4\n')
    def test_binary_source_mismatch_keeps_existing_version(self):
        (self.assets/'version.txt').write_text('9.9.9\n');self.run_failure()
        self.assertEqual((self.install/'version.txt').read_text(),'1.2.4\n')
    def test_successful_launcher_without_new_heartbeat_is_not_success(self):
        self.write(self.assets/'start.sh','#!/bin/sh\nexit 0\n')
        self.write(self.bin/'pidof','#!/bin/sh\necho 1234\n')
        status=self.run_failure();self.assertIn('not verified',status['message'])
    def test_failed_start_never_reports_success(self):
        status=self.run_failure();self.assertIn('startup failed',status['message'])
if __name__=='__main__':unittest.main()
