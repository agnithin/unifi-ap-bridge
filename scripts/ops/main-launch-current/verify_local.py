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
recovery_sha = '33c26d45a8cc05df015625e3aab42c29857cf09459a3dd4d3e7548ca31f70f9c'
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
launch_payload = launch_command.rstrip('\n')  # POSIX command substitution removes trailing newlines.
recovery_payload = recovery_command.rstrip('\n')
launch_handoff = subprocess.check_output(['python3', str(root/'build_command.py'), '--handoff', '/tmp/launch-handoff-test', hashlib.sha256(launch_payload.encode()).hexdigest()], text=True)
recovery_handoff = subprocess.check_output(['python3', str(root/'build_command.py'), '--handoff', '/tmp/recovery-handoff-test', hashlib.sha256(recovery_payload.encode()).hexdigest()], text=True)
assert len(launch_handoff) < 1000 and len(recovery_handoff) < 1000
for wrapper in ['launch_main.sh', 'recover_main.sh']:
    wrapper_text = (root/wrapper).read_text()
    assert "printf '%s' \"$REMOTE_COMMAND\" | rtk proxy ssh" in wrapper_text
    assert '--handoff "$HANDOFF_DIR" "$REMOTE_SHA"' in wrapper_text
with tempfile.TemporaryDirectory(prefix='main-current-packet-') as td:
    td = pathlib.Path(td)
    for name, command in [('launch.payload', launch_payload), ('recovery.payload', recovery_payload),
                          ('launch.receiver', launch_handoff), ('recovery.receiver', recovery_handoff)]:
        path = td / name
        path.write_text(command)
        subprocess.run(['sh', '-n', str(path)], check=True)
    # Exercise the verified stdin receiver locally with a harmless sentinel.
    sentinel = b"#!/bin/sh\nprintf '%s\\n' 'safe quote: a'\\''b' > sentinel.result\n"
    sentinel_sha = hashlib.sha256(sentinel).hexdigest()
    sentinel_dir = td/'sentinel-handoff'
    sentinel_receiver = subprocess.check_output(['python3', str(root/'build_command.py'), '--handoff', str(sentinel_dir), sentinel_sha], text=True)
    subprocess.run(['sh', '-c', sentinel_receiver], cwd=td, input=sentinel, check=True, stdout=subprocess.DEVNULL)
    assert not sentinel_dir.exists()
    assert (td/'sentinel.result').read_text() == "safe quote: a'b\n"
    (td/'sentinel.result').unlink()
    truncated_dir = td/'truncated-handoff'
    truncated_receiver = subprocess.check_output(['python3', str(root/'build_command.py'), '--handoff', str(truncated_dir), sentinel_sha], text=True)
    truncated = subprocess.run(['sh', '-c', truncated_receiver], cwd=td, input=sentinel[:-1], stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    assert truncated.returncode != 0 and b'handoff checksum mismatch' in truncated.stderr
    assert (truncated_dir/'script.sh').exists() and not (td/'sentinel.result').exists()

# Exercise actual preservation helpers with synthetic non-secret data.
compare_fn = re.search(r'(?ms)^compare\(\) \{\n.*?^\}', launch_template).group(0)
identity_fn = re.search(r'(?ms)^state_identity\(\) \{\n.*?^\}', launch_template).group(0)
locks_fn = re.search(r'(?ms)^locks_absent\(\) \{\n.*?^\}', launch_template).group(0)
uint_fn = re.search(r'(?ms)^uint\(\) \{\n.*?^\}', launch_template).group(0)
status_fresh_fn = re.search(r'(?ms)^status_is_fresh\(\) \{\n.*?^\}', launch_template).group(0)
restart_fresh_fn = re.search(r'(?ms)^fresh_inform_after_restart\(\) \{\n.*?^\}', launch_template).group(0)
recovery_template = (root/'recover.remote.sh').read_text()
assert uint_fn == re.search(r'(?ms)^uint\(\) \{\n.*?^\}', recovery_template).group(0)
assert status_fresh_fn == re.search(r'(?ms)^status_is_fresh\(\) \{\n.*?^\}', recovery_template).group(0)
assert restart_fresh_fn == re.search(r'(?ms)^fresh_inform_after_restart\(\) \{\n.*?^\}', recovery_template).group(0)
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
    # Epoch values are UTC-independent. The daemon's zone-less heartbeat text
    # is deliberately never compared with the shell's local wall-clock text.
    start = 1792994494  # 2026-10-26 06:01:34 UTC; local shell was 2026-10-25 23:01:34 -0700.
    probe = f'''{uint_fn}\n{status_fresh_fn}\n{restart_fresh_fn}\nstatus_is_fresh 1792994499 1792994494 45 || exit 38\nif status_is_fresh 1792998094 1792994494 45; then exit 39; fi\nif status_is_fresh 1792994499 1792994500 45; then exit 40; fi\nif status_is_fresh invalid 1792994494 45; then exit 41; fi\n'''
    probe += f'''fresh_inform_after_restart {start+2} {start} {start+5} 45 812 812 '2026-10-26 06:00:00' '{commit}' '{commit}' 'Active (GCM)' || exit 42\n'''
    # Regression for the reviewed timezone bug: a pre-start UTC heartbeat
    # looks later than the prior-day local start string, but its daemon status
    # epoch is before launch and must fail closed.
    probe += f'''if fresh_inform_after_restart {start-10} {start} {start+5} 45 812 812 '2026-10-26 06:01:24' '{commit}' '{commit}' 'Active (GCM)'; then exit 43; fi\n'''
    probe += f'''if fresh_inform_after_restart {start} {start} {start+5} 45 812 812 '2026-10-26 06:01:34' '{commit}' '{commit}' 'Active (GCM)'; then exit 44; fi\n'''
    probe += f'''if fresh_inform_after_restart {start+2} {start} {start+5} 45 812 811 '2026-10-26 06:01:36' '{commit}' '{commit}' 'Active (GCM)'; then exit 45; fi\n'''
    probe += f'''if fresh_inform_after_restart {start+2} {start} {start+5} 45 812 812 '' '{commit}' '{commit}' 'Active (GCM)'; then exit 46; fi\n'''
    probe += f'''if fresh_inform_after_restart {start+2} {start} {start+60} 45 812 812 '2026-10-26 06:01:36' '{commit}' '{commit}' 'Active (GCM)'; then exit 47; fi\n'''
    probe += f'''if fresh_inform_after_restart {start+2} {start} {start+5} 45 812 812 '2026-10-26 06:01:36' '{commit}' '{commit}' 'Error: timeout'; then exit 48; fi\n'''
    subprocess.run(['sh', '-eu', '-c', probe], check=True)
print(f'candidate={commit}; armv7_archive_sha256={archive_sha}; sidecar_sha256={sidecar_sha}; updater_sha256={updater_sha}')
print('release manifest, all member hashes, architecture, exact member set and sidecar: PASS')
print(f'launch_stream_bytes={len(launch_command)}; recovery_stream_bytes={len(recovery_command)}; receiver_command_bytes={len(launch_handoff)}/{len(recovery_handoff)}; shell_syntax=PASS')
print('verified same-session stdin handoff, truncated-stream rejection, preservation comparators, state identity, lock cleanup, UTC/local-boundary and stale-heartbeat regressions: PASS')
