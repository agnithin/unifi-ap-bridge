#!/usr/bin/env python3
"""Local fixtures for the RAM-only Main updater-path diagnostic."""
import os
import pathlib
import re
import shlex
import subprocess
import tempfile
import prepare_packet

root = pathlib.Path(__file__).resolve().parent
source = (root / 'diagnose.remote.sh').read_text()
match = re.search(r'(?ms)^observe\(\) \{\n.*?^\}', source)
assert match, 'observe() function not found'
observe = match.group(0)
sha = re.search(r'(?m)^sha256\(\) \{.*$', source)
assert sha

def run_probe(base, label, path, sample, env=None, expect_failure=False):
    stage = base
    log = base / 'observations.log'
    script = f'''STAGE={str(stage)!r}
LOG={str(log)!r}
{sha.group(0)}
fail() {{ printf 'fixture_stop=%s\\n' "$1"; exit 70; }}
{observe}
observe {label} {str(path)!r} {sample}
'''
    result = subprocess.run(['sh', '-eu', '-c', script], check=False, env=env,
                            capture_output=True, text=True)
    assert (result.returncode == 70) == expect_failure, (label, result.returncode)
    if expect_failure:
        assert 'fixture_stop=observation_' in result.stdout
    return log.read_text().splitlines()[-1]

with tempfile.TemporaryDirectory(prefix='ramdiag-fixtures-') as temp:
    base = pathlib.Path(temp)
    (base / 'observations.log').write_text('')

    regular = base / 'regular'; regular.write_bytes(b'fixture bytes only\n'); regular.chmod(0o700)
    row = run_probe(base, 'regular', regular, 'immediate')
    assert 'state=present type=regular mode=-rwx------ size=19 sha256=' in row
    assert 'fixture bytes only' not in row

    link = base / 'link'; link.symlink_to(regular.name)
    row = run_probe(base, 'link', link, 'immediate')
    assert 'state=present type=symlink mode=lrwx' in row and 'sha256=unknown' in row

    missing = base / 'missing'
    row = run_probe(base, 'missing', missing, 'later')
    assert 'state=absent_confirmed' in row

    missing_parent = base / 'gone' / 'missing'
    row = run_probe(base, 'missing-parent', missing_parent, 'later', expect_failure=True)
    assert 'state=observation_error' in row and 'absent_confirmed' not in row

    fail_bin = base / 'fail-bin'; fail_bin.mkdir()
    (fail_bin / 'ls').write_text('#!/bin/sh\nexit 2\n'); (fail_bin / 'ls').chmod(0o755)
    row = run_probe(base, 'list-error', base / 'not-there', 'later',
                    {**os.environ, 'PATH': str(fail_bin) + ':' + os.environ['PATH']}, expect_failure=True)
    assert 'state=observation_error' in row

    fail_bin_grep = base / 'fail-bin-grep'; fail_bin_grep.mkdir()
    (fail_bin_grep / 'grep').write_text('#!/bin/sh\nexit 2\n'); (fail_bin_grep / 'grep').chmod(0o755)
    row = run_probe(base, 'grep-error', base / 'not-there', 'later',
                    {**os.environ, 'PATH': str(fail_bin_grep) + ':' + os.environ['PATH']}, expect_failure=True)
    assert 'state=observation_error' in row and 'absent_confirmed' not in row

    fail_bin2 = base / 'fail-bin-2'; fail_bin2.mkdir()
    (fail_bin2 / 'openssl').write_text('#!/bin/sh\nexit 2\n'); (fail_bin2 / 'openssl').chmod(0o755)
    row = run_probe(base, 'read-error', regular, 'later',
                    {**os.environ, 'PATH': str(fail_bin2) + ':' + os.environ['PATH']}, expect_failure=True)
    assert 'state=read_error' in row and 'absent_confirmed' not in row

    neutral = base / 'neutral'; neutral.write_bytes(b'fixture bytes only\n')
    neutral.chmod(0o700)
    regular.unlink()
    disappeared = run_probe(base, 'updater-name', regular, 'later')
    remains = run_probe(base, 'neutral-name', neutral, 'later')
    assert 'state=absent_confirmed' in disappeared
    assert 'state=present type=regular' in remains
    retained = (base / 'observations.log').read_text()
    assert 'fixture bytes only' not in retained

receiver = (root / 'prepare_packet.py').read_text()
firmware_source = source + receiver
for required in ('hostname', 'pidof', 'date', 'sed', 'grep', 'df', 'mkdir', 'cat',
                 'openssl', 'awk', 'tr', 'cp', 'wc', 'ls', 'tar', 'sleep', 'head'):
    assert re.search(rf'\b{required}\b', firmware_source), f'missing documented firmware command: {required}'
for unsupported_or_uncertain in ('stat ', 'readlink ', 'find ', 'timeout ', 'nohup ', 'chmod '):
    assert unsupported_or_uncertain not in source, unsupported_or_uncertain
assert 'apply-exact' not in source and 'candidate-check' not in source
assert '/jffs' not in source and 'unifi-bridge/unifi-bridge' not in source
assert 'case-extract' in source and 'tar -xzpf' in source
assert 'case-updater/update.sh' in source and 'case-neutral/payload.bin' in source
assert 'case-control/update.sh' in source
assert 'sleep 60' not in source
assert source.index("printf 'extraction=%s\\n' \"$extraction_result\"") < source.index('observe extracted-updater')
assert source.index('observe extracted-updater') < source.index('[ "$extraction_result" = success ]')
assert 'chmod ' not in source and '/jffs' not in source
command = prepare_packet.remote_command('202610261845301238', 'a' * 64)
parts = shlex.split(command)
assert parts[:2] == ['/bin/sh', '-c']
receiver = parts[2]
subprocess.run(['sh', '-n', '-c', receiver], check=True)
assert "'\"commit\": \"be106ec33fe6790af30a600d11bd05246445c530\"'" in receiver
assert "'\"adopted\": true'" in receiver
with tempfile.TemporaryDirectory(prefix='ramdiag-receiver-fixture-') as temp:
    status_fixture = pathlib.Path(temp) / 'status.js'
    status_fixture.write_text('''{"commit": "be106ec33fe6790af30a600d11bd05246445c530", "adopted": true, "running": true, "mode": "GCM", "last_status": "Active (GCM)"}''')
    loop = re.search(r'(?ms)^for expected in .*?^done$', receiver)
    assert loop
    probe = f'''STATUS={shlex.quote(str(status_fixture))}
BASE_COMMIT='be106ec33fe6790af30a600d11bd05246445c530'
fail() {{ exit 70; }}
{loop.group(0)}
'''
    subprocess.run(['sh', '-eu', '-c', probe], check=True)
print('RAM diagnostic fixtures: regular, symlink, confirmed absence, parent/list observation error, read/hash error, disappearance versus neutral control, sanitized rows and firmware-command guard: PASS')
