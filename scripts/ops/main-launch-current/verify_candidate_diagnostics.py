#!/usr/bin/env python3
"""Exercise sanitized postflight comparison classifications with local fixtures."""
import os
import pathlib
import re
import subprocess
import tempfile

root = pathlib.Path(__file__).resolve().parent
template = (root / 'launch.remote.template.sh').read_text()

def function(name, pattern):
    match = re.search(pattern, template)
    assert match, name
    return match.group(0)

mode_fn = function('perm', r'(?m)^perm\(\)[^\n]*$')
file_state_fn = function('file_state', r'(?ms)^file_state\(\) \{\n.*?^\}')
readable_fn = function('file_readable', r'(?m)^file_readable\(\)[^\n]*$')
compare_fn = function('compare_candidate_file', r'(?ms)^compare_candidate_file\(\) \{\n.*?^\}')
compare_mode_fn = function('compare_candidate_mode', r'(?ms)^compare_candidate_mode\(\) \{\n.*?^\}')
record_fn = function('record_candidate_file', r'(?ms)^record_candidate_file\(\) \{\n.*?^\}')
milestone_fn = function('candidate_milestone', r'(?ms)^candidate_milestone\(\) \{\n.*?^\}')
helpers = '\n'.join([mode_fn, file_state_fn, readable_fn, compare_fn, compare_mode_fn])

with tempfile.TemporaryDirectory(prefix='candidate-diagnostics-') as temp:
    base = pathlib.Path(temp)
    source, destination = base/'candidate', base/'installed'
    source.write_text('secret fixture bytes\n')
    destination.write_text('secret fixture bytes\n')
    source.chmod(0o700); destination.chmod(0o700)
    unreadable = base/'unreadable-fixture'
    unreadable.write_text('synthetic only\n')
    blocked = f'''{helpers}
blocked={unreadable}
file_readable() {{ [ "$1" != "$blocked" ] && [ -r "$1" ]; }}
[ "$(compare_candidate_file {source} {destination})" = same ] || exit 50
[ "$(compare_candidate_file {base/'missing-source'} {destination})" = source_missing ] || exit 51
[ "$(compare_candidate_file {source} {base/'missing-destination'})" = destination_missing ] || exit 52
[ "$(compare_candidate_file "$blocked" {destination})" = source_unreadable ] || exit 53
[ "$(compare_candidate_file {source} "$blocked")" = destination_unreadable ] || exit 59
[ "$(compare_candidate_mode {source} {destination})" = same ] || exit 54
'''
    subprocess.run(['sh', '-eu', '-c', blocked], check=True)

    destination.write_text('changed fixture bytes\n')
    changed = helpers + f'''\n[ "$(compare_candidate_file {source} {destination})" = different ] || exit 55
'''
    subprocess.run(['sh', '-eu', '-c', changed], check=True)
    destination.chmod(0o600)
    mode_changed = helpers + f'''\n[ "$(compare_candidate_mode {source} {destination})" = different ] || exit 56
'''
    subprocess.run(['sh', '-eu', '-c', mode_changed], check=True)

    cmp_bin = base/'cmp-bin'; cmp_bin.mkdir()
    cmp_shim = cmp_bin/'cmp'; cmp_shim.write_text('#!/bin/sh\nexit 2\n'); cmp_shim.chmod(0o755)
    cmp_error = helpers + f'''\n[ "$(compare_candidate_file {source} {destination})" = comparison_error ] || exit 57
'''
    subprocess.run(['sh', '-eu', '-c', cmp_error], check=True,
                   env={**os.environ, 'PATH': str(cmp_bin)+':'+os.environ['PATH']})

    (base/'candidate-files.status').write_text('file content=code mode=code\n')
    fixed_mode_path = base/'fixed-mode-file'
    fixed_mode_path.write_text('fixed mode fixture\n'); fixed_mode_path.chmod(0o644)
    record_probe = f'''STAGE={base}
{helpers}
{record_fn}
if record_candidate_file synthetic {base/'missing-source'} {destination} compare; then exit 58; fi
record_candidate_file fixed-same {fixed_mode_path} {fixed_mode_path} '-rw-r--r--' || exit 60
if record_candidate_file fixed-missing {fixed_mode_path} {base/'missing-fixed-destination'} '-rw-r--r--'; then exit 61; fi
'''
    subprocess.run(['sh', '-eu', '-c', record_probe], check=True)
    retained = (base/'candidate-files.status').read_text()
    assert 'synthetic content=source_missing mode=source_mode_error' in retained
    assert 'fixed-same content=same mode=same' in retained
    assert 'fixed-missing content=destination_missing mode=destination_mode_error' in retained
    assert 'secret fixture bytes' not in retained and 'changed fixture bytes' not in retained

    (base/'candidate-milestones.status').write_text('archive=not_checked\n')
    milestone_probe = f'''STAGE={base}
{milestone_fn}
candidate_milestone archive present
'''
    subprocess.run(['sh', '-eu', '-c', milestone_probe], check=True)
    assert (base/'candidate-milestones.status').read_text() == 'archive=present\n'

assert 'cmp -s "$1" "$2"' in compare_fn
assert 'source_missing' in compare_fn and 'destination_missing' in compare_fn
assert 'source_unreadable' in compare_fn and 'destination_unreadable' in compare_fn
assert 'comparison_error' in compare_fn
assert 'candidate-files.status' in template and 'candidate-milestones.status' in template
print('candidate comparison fixtures: same, missing source/destination, unreadable source/destination, cmp error, content difference, mode difference/error, sanitized retention and milestone update: PASS')
