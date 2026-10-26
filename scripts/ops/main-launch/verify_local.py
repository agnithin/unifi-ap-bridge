import hashlib
import json
import pathlib
import re
import subprocess
import tarfile
import tempfile

root = pathlib.Path(__file__).resolve().parent
repo = root.parents[2]
source = repo / 'scripts/update.sh'
expected_script = '2f2dc19723cf442a2ff5ea7ebd9e8b10b56b6a0c24ab2441eca602c14407547a'
assert hashlib.sha256(source.read_bytes()).hexdigest() == expected_script
commit_script = subprocess.check_output(['git', 'show', 'a25bb6b272e6dcbe938b61fbca8d17044c6ff07d:scripts/update.sh'], cwd=repo)
assert hashlib.sha256(commit_script).hexdigest() == expected_script
for arch, digest in {
    'armv7': 'cdd11384e69da61775b75fe43b4b9d48bba54b4984f39c516f757128e23422a5',
    'arm64': '279734369557ae812f6a909755c93789e028c78ae20dd8bc868ccdaee54ac757',
}.items():
    bundle = pathlib.Path(f'/private/tmp/unifi-w7-a25bb6b/unifi-bridge-addon-a25bb6b272e6dcbe938b61fbca8d17044c6ff07d-{arch}.tar.gz')
    assert hashlib.sha256(bundle.read_bytes()).hexdigest() == digest
    sidecar = pathlib.Path(str(bundle) + '.sha256')
    checksum_rows = sidecar.read_text().splitlines()
    assert len(checksum_rows) == 1
    checksum, name = checksum_rows[0].split()
    assert checksum == digest and name == bundle.name
    with tarfile.open(bundle, 'r:gz') as tf:
        manifest = json.load(tf.extractfile('bundle.json'))
        assert manifest['commit'] == 'a25bb6b272e6dcbe938b61fbca8d17044c6ff07d'
        assert manifest['version'] == '1.2.5'
        assert manifest['architecture'] == arch
        embedded = tf.extractfile('update.sh').read()
        assert hashlib.sha256(embedded).hexdigest() == expected_script
        members = sorted(x.name for x in tf.getmembers())
        assert members == sorted(['apply_config.sh','bundle.json','install.sh','start.sh','sync_nodes.sh',f'unifi-bridge-{arch}','unifi-bridge.asp','unifi-bridge.sh','update.sh','version.txt','watchdog.sh'])
subprocess.run(['sh', '-n', str(source)], check=True)
command = subprocess.check_output(['python3', str(root/'build_command.py'), str(root/'launch.remote.template.sh'), str(source)], text=True)
assert len(command) < 110000
cmdfile = root/'generated.remote.command'
cmdfile.write_text(command)
subprocess.run(['sh', '-n', str(cmdfile)], check=True)
recovery = root/'recover.remote.sh'
expected_recovery = '8b73dbd5d08229a5eefea310969abe60d33b04f83e33196cc987138b50989cae'
assert hashlib.sha256(recovery.read_bytes()).hexdigest() == expected_recovery
subprocess.run(['sh', '-n', str(recovery)], check=True)
recovery_command = subprocess.check_output(['python3', str(root/'build_command.py'), str(root/'recover.remote.template.sh'), str(recovery)], text=True)
assert len(recovery_command) < 110000
recovery_cmdfile = root/'generated.recovery.command'
recovery_cmdfile.write_text(recovery_command)
subprocess.run(['sh', '-n', str(recovery_cmdfile)], check=True)
# Exercise the same POSIX single-quote serializer locally with a harmless script.
sentinel = b"#!/bin/sh\nprintf '%s\\n' 'safe quote: a'\\''b' > /tmp/unifi-handoff-local-sentinel.result\n"
literal = "'" + sentinel.decode().replace("'", "'\\''") + "'"
target = '/tmp/unifi-handoff-local-sentinel.sh'
local_command = f"printf '%s' {literal} > {target} && chmod 700 {target} && openssl dgst -sha256 {target} && sh -n {target} && /bin/sh {target}"
subprocess.run(['sh', '-c', local_command], check=True)
written = pathlib.Path(target).read_bytes()
assert written == sentinel
assert pathlib.Path('/tmp/unifi-handoff-local-sentinel.result').read_text() == "safe quote: a'b\n"
# Exercise the packet's actual preservation comparators on synthetic local data.
template = (root/'launch.remote.template.sh').read_text()
compare_fn = re.search(r'(?ms)^compare\(\) \{\n.*?^\}', template).group(0)
identity_fn = re.search(r'(?ms)^state_identity\(\) \{\n.*?^\}', template).group(0)
with tempfile.TemporaryDirectory(prefix='main-packet-check-') as td:
    td = pathlib.Path(td)
    a, b, c = (td/name for name in ('a','b','c'))
    a.write_text('same\n'); b.write_text('same\n'); c.write_text('different\n')
    probe = f'''{compare_fn}\n[ "$(compare {a} {b})" = same ] || exit 31\n[ "$(compare {a} {c})" = changed ] || exit 32\n[ "$(compare {a} {td/'missing'})" = error ] || exit 33\n'''
    subprocess.run(['sh', '-eu', '-c', probe], check=True)
    state1 = td/'state1.json'; state2 = td/'state2.json'; state_bad = td/'state_bad.json'
    state1.write_text('{"schema":1,"mac":"00:11:22:33:44:55","adopted":true,"mode":"GCM","authkey":"test-only-placeholder","paused":false,"metadata":1}\n')
    state2.write_text('{"schema":1,"mac":"00:11:22:33:44:55","adopted":true,"mode":"GCM","authkey":"test-only-placeholder","paused":false,"metadata":2}\n')
    state_bad.write_text('{"schema":1,"mac":"00:11:22:33:44:55","adopted":true,"mode":"GCM","paused":false}\n')
    norm1 = td/'norm1'; norm2 = td/'norm2'; norm_bad = td/'norm_bad'
    probe = f'''{identity_fn}\nstate_identity {state1} {norm1}\nstate_identity {state2} {norm2}\ncmp -s {norm1} {norm2} || exit 34\nif state_identity {state_bad} {norm_bad}; then exit 35; fi\n'''
    subprocess.run(['sh', '-eu', '-c', probe], check=True)
print(f'script_sha256={expected_script}')
print('armv7_artifact=verified; arm64_artifact=verified; metadata/member sets=verified')
print(f'generated_remote_command_bytes={len(command)}; shell_syntax=ok')
print(f'generated_recovery_command_bytes={len(recovery_command)}; recovery_sha256={expected_recovery}; shell_syntax=ok')
print('local_same_session_printf_roundtrip=ok; sentinel_only')
print('preservation_compare_and_state_identity_regressions=ok; synthetic_local_data')
