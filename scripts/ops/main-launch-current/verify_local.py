#!/usr/bin/env python3
import hashlib
import json
import pathlib
import re
import subprocess
import sys
import tarfile
import tempfile

root = pathlib.Path(__file__).resolve().parent
repo = root.parents[2]
commit = '1f22343206c80bda01d576dfdb813e4a911e4568'
archive_sha = '16300c4bc0f08f6d5bfadae8427a7928f01de1470eaf7d000c317be6d27657f1'
sidecar_sha = '113a1ffc283a5a27ba1594736d827088aacab9e7936478234e0c775677eaf48f'
updater_sha = '2f2dc19723cf442a2ff5ea7ebd9e8b10b56b6a0c24ab2441eca602c14407547a'
recovery_sha = 'f3b4c00e5ea578a2f0da8fe8144dab5000f694b247b7ac0a321876fd4572ebef'
archive = pathlib.Path(sys.argv[1]) if len(sys.argv) > 1 else pathlib.Path('/private/tmp/unifi-main-1f223-current-armv7.tar.gz')
sidecar = pathlib.Path(sys.argv[2]) if len(sys.argv) > 2 else pathlib.Path(str(archive) + '.sha256')

def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()

assert sha(archive) == archive_sha
assert sha(sidecar) == sidecar_sha
rows = sidecar.read_text().splitlines()
assert rows == [f'{archive_sha}  unifi-bridge-addon-{commit}-armv7.tar.gz']
source = repo / 'scripts/update.sh'
assert sha(source) == updater_sha
assert hashlib.sha256(subprocess.check_output(['git', 'show', f'{commit}:scripts/update.sh'], cwd=repo)).hexdigest() == updater_sha
with tarfile.open(archive, 'r:gz') as tf:
    expected = sorted(['apply_config.sh', 'bundle.json', 'install.sh', 'start.sh', 'sync_nodes.sh',
                       'unifi-bridge-armv7', 'unifi-bridge.asp', 'unifi-bridge.sh', 'update.sh',
                       'version.txt', 'watchdog.sh'])
    members = [m.name for m in tf.getmembers()]
    assert sorted(members) == expected and len(members) == len(expected)
    manifest = json.load(tf.extractfile('bundle.json'))
    assert manifest['commit'] == commit and manifest['version'] == '1.2.5' and manifest['architecture'] == 'armv7'
    assert set(manifest) == {'version', 'commit', 'architecture'} | {f'sha256_{name}' for name in expected if name != 'bundle.json'}
    for name in expected:
        if name == 'bundle.json':
            continue
        payload = tf.extractfile(name).read()
        assert hashlib.sha256(payload).hexdigest() == manifest[f'sha256_{name}'], name
    assert hashlib.sha256(tf.extractfile('update.sh').read()).hexdigest() == updater_sha

for name in ['launch_main.sh', 'recover_main.sh', 'launch.remote.template.sh', 'recover.remote.template.sh', 'recover.remote.sh']:
    subprocess.run(['sh', '-n', str(root / name)], check=True)
assert sha(root / 'recover.remote.sh') == recovery_sha
for wrapper, expected_sha in [('launch_main.sh', updater_sha), ('recover_main.sh', recovery_sha)]:
    assert f"EXPECTED='{expected_sha}'" in (root / wrapper).read_text()
launch_template = (root / 'launch.remote.template.sh').read_text()
assert f"SCRIPT_SHA='{updater_sha}'" in launch_template and archive_sha in launch_template
launch_command = subprocess.check_output(['python3', str(root/'build_command.py'), str(root/'launch.remote.template.sh'), str(source)], text=True)
recovery_command = subprocess.check_output(['python3', str(root/'build_command.py'), str(root/'recover.remote.template.sh'), str(root/'recover.remote.sh')], text=True)
assert len(launch_command) < 110000 and len(recovery_command) < 110000
with tempfile.TemporaryDirectory(prefix='main-current-packet-') as td:
    td = pathlib.Path(td)
    for name, command in [('launch.command', launch_command), ('recovery.command', recovery_command)]:
        path = td / name
        path.write_text(command)
        subprocess.run(['sh', '-n', str(path)], check=True)
    # Exercise the exact POSIX single-quote transport shape with a harmless sentinel.
    sentinel = b"#!/bin/sh\nprintf '%s\\n' 'safe quote: a'\\''b' > sentinel.result\n"
    quoted = "'" + sentinel.decode().replace("'", "'\\''") + "'"
    remote = f"printf '%s' {quoted} > {td}/sentinel.sh && chmod 700 {td}/sentinel.sh && openssl dgst -sha256 {td}/sentinel.sh && sh -n {td}/sentinel.sh && /bin/sh {td}/sentinel.sh"
    subprocess.run(['sh', '-c', remote], cwd=td, check=True, stdout=subprocess.DEVNULL)
    assert (td/'sentinel.sh').read_bytes() == sentinel
    assert (td/'sentinel.result').read_text() == "safe quote: a'b\n"

# Exercise actual preservation helpers with synthetic non-secret data.
compare_fn = re.search(r'(?ms)^compare\(\) \{\n.*?^\}', launch_template).group(0)
identity_fn = re.search(r'(?ms)^state_identity\(\) \{\n.*?^\}', launch_template).group(0)
locks_fn = re.search(r'(?ms)^locks_absent\(\) \{\n.*?^\}', launch_template).group(0)
fresh_fn = re.search(r'(?ms)^inform_is_fresh\(\) \{\n.*?^\}', launch_template).group(0)
with tempfile.TemporaryDirectory(prefix='main-current-preservation-') as td:
    td = pathlib.Path(td)
    same, changed, absent = td/'same', td/'changed', td/'absent'
    same.write_text('same\n'); changed.write_text('different\n')
    probe = f'''{compare_fn}\n[ "$(compare {same} {same})" = same ] || exit 31\n[ "$(compare {same} {changed})" = changed ] || exit 32\n[ "$(compare {same} {absent})" = error ] || exit 33\n'''
    subprocess.run(['sh', '-eu', '-c', probe], check=True)
    state_a, state_b, state_bad = td/'state-a', td/'state-b', td/'state-bad'
    state_a.write_text('{"schema":1,"mac":"00:11:22:33:44:55","adopted":true,"mode":"GCM","authkey":"test-only","paused":false,"metadata":1}\n')
    state_b.write_text('{"schema":1,"mac":"00:11:22:33:44:55","adopted":true,"mode":"GCM","authkey":"test-only","paused":false,"metadata":2}\n')
    state_bad.write_text('{"schema":1,"mac":"00:11:22:33:44:55","adopted":true,"mode":"GCM","paused":false}\n')
    n1, n2, nb = td/'n1', td/'n2', td/'nb'
    probe = f'''{identity_fn}\nstate_identity {state_a} {n1}\nstate_identity {state_b} {n2}\ncmp -s {n1} {n2} || exit 34\nif state_identity {state_bad} {nb}; then exit 35; fi\n'''
    subprocess.run(['sh', '-eu', '-c', probe], check=True)
    lock_a, lock_b = td/'lock-a', td/'lock-b'
    (lock_a).mkdir()
    probe = f'''{locks_fn}\nlocks_absent {td}/nope {td}/also-nope || exit 36\nif locks_absent {lock_a}; then exit 37; fi\n'''
    subprocess.run(['sh', '-eu', '-c', probe], check=True)
    probe = f'''{fresh_fn}\ninform_is_fresh '2026-10-26 05:47:00' '2026-10-26 05:46:00' '2026-10-26 05:46:30' || exit 38\nif inform_is_fresh '2026-10-26 05:46:00' '2026-10-26 05:46:00' '2026-10-26 05:45:00'; then exit 39; fi\nif inform_is_fresh '2026-10-26 05:46:00' '2026-10-26 05:45:00' '2026-10-26 05:46:30'; then exit 40; fi\n'''
    subprocess.run(['sh', '-eu', '-c', probe], check=True)
print(f'candidate={commit}; armv7_archive_sha256={archive_sha}; sidecar_sha256={sidecar_sha}; updater_sha256={updater_sha}')
print('release manifest, all member hashes, architecture, exact member set and sidecar: PASS')
print(f'launch_command_bytes={len(launch_command)}; recovery_command_bytes={len(recovery_command)}; shell_syntax=PASS')
print('same-session sentinel, preservation comparators, state identity, lock cleanup and fresh-inform regressions: PASS')
