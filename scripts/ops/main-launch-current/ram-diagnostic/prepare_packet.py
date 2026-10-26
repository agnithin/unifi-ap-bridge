#!/usr/bin/env python3
"""Build a verified local tar stream and its one-session /tmp-only receiver command."""
import argparse
import hashlib
import io
import os
import pathlib
import re
import secrets
import shlex
import tarfile
import time

COMMIT = '1f22343206c80bda01d576dfdb813e4a911e4568'
ARCHIVE_SHA = '16300c4bc0f08f6d5bfadae8427a7928f01de1470eaf7d000c317be6d27657f1'
SIDECAR_SHA = '113a1ffc283a5a27ba1594736d827088aacab9e7936478234e0c775677eaf48f'
UPDATER_SHA = '2f2dc19723cf442a2ff5ea7ebd9e8b10b56b6a0c24ab2441eca602c14407547a'
BASE_COMMIT = 'be106ec33fe6790af30a600d11bd05246445c530'
HOST = 'nithin@10.100.1.10'

def digest(data):
    return hashlib.sha256(data).hexdigest()

def add_bytes(tf, name, data):
    info = tarfile.TarInfo(name)
    info.size = len(data)
    info.mode = 0o600
    info.mtime = 0
    tf.addfile(info, io.BytesIO(data))

def remote_command(request_id, bundle_sha):
    stage = f'/tmp/unifi-main-ramdiag-{COMMIT}-{request_id}'
    # This preflight reads only identity and sanitized daemon status. Its sole
    # remote writes are the unique private /tmp stage and transferred payload.
    body = f'''set -eu
umask 077
HOST_EXPECTED='ZenWiFi_XT8-7B10'
BASE_COMMIT='{BASE_COMMIT}'
STATUS='/tmp/var/wwwext/unifi-bridge/status.js'
STAGE='{stage}'
fail() {{ printf 'receiver=STOP reason=%s\\n' "$1"; exit 70; }}
[ "$(hostname 2>/dev/null)" = "$HOST_EXPECTED" ] || fail host_identity
pids=$(pidof unifi-bridge 2>/dev/null | tr -s '[:space:]' ' ' | sed 's/^ //;s/ $//')
case "$pids" in ''|*[!0-9\\ ]*) fail daemon_pid_list;; esac
set -- $pids
[ "$#" -eq 1 ] || fail daemon_pid_count
pid=$1
[ -r "$STATUS" ] || fail status_unreadable
status_pid=$(sed -n 's/.*"pid":[[:space:]]*\\([0-9][0-9]*\\).*/\\1/p' "$STATUS" | head -n 1)
updated=$(sed -n 's/.*"status_updated_at":[[:space:]]*\\([0-9][0-9]*\\).*/\\1/p' "$STATUS" | head -n 1)
now=$(date +%s)
case "$updated:$now" in *[!0-9:]*) fail status_timestamp;; esac
[ "$status_pid" = "$pid" ] && [ "$updated" -le "$now" ] && [ $((now - updated)) -le 45 ] || fail stale_status
for expected in {shlex.quote('"commit": "' + BASE_COMMIT + '"')} '"adopted": true' '"running": true' '"mode": "GCM"' '"last_status": "Active (GCM)"'; do
  grep -Fq "$expected" "$STATUS" || fail daemon_health
done
available_kb=$(df -k /tmp | awk 'END {{print $4}}')
case "$available_kb" in ''|*[!0-9]*) fail tmp_space_query;; esac
[ "$available_kb" -ge 20480 ] || fail tmp_space_below_20MiB
[ ! -e "$STAGE" ] && [ ! -L "$STAGE" ] || fail unique_stage_exists
mkdir -m 700 "$STAGE" || fail stage_create
cat > "$STAGE/packet.tar.gz" || fail payload_transfer
actual=$(openssl dgst -sha256 "$STAGE/packet.tar.gz" | awk '{{print $NF}}') || fail payload_hash_read
[ "$actual" = '{bundle_sha}' ] || fail payload_hash
mkdir -m 700 "$STAGE/payload" || fail payload_dir
tar -xzpf "$STAGE/packet.tar.gz" -C "$STAGE/payload" || fail payload_extract
printf 'receiver=PASS host=Main pid=%s status_age_sec=%s stage=%s\\n' "$pid" "$((now - updated))" "$STAGE"
/bin/sh "$STAGE/payload/diagnose.remote.sh" "$STAGE"
'''
    return '/bin/sh -c ' + shlex.quote(body)

def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--archive', required=True, type=pathlib.Path)
    p.add_argument('--sidecar', required=True, type=pathlib.Path)
    p.add_argument('--updater', required=True, type=pathlib.Path)
    p.add_argument('--output', required=True, type=pathlib.Path)
    p.add_argument('--request-id', help='fresh numeric id; generated if omitted')
    args = p.parse_args()
    args.request_id = args.request_id or f'{int(time.time())}{os.getpid():05d}{secrets.randbelow(10000):04d}'
    if not re.fullmatch(r'[0-9]{14,20}', args.request_id):
        p.error('request-id must be a fresh 14–20 digit value')
    archive = args.archive.read_bytes()
    sidecar = args.sidecar.read_bytes()
    updater = args.updater.read_bytes()
    if digest(archive) != ARCHIVE_SHA:
        p.error('candidate archive SHA-256 mismatch')
    if digest(sidecar) != SIDECAR_SHA:
        p.error('candidate sidecar SHA-256 mismatch')
    expected_row = f'{ARCHIVE_SHA}  unifi-bridge-addon-{COMMIT}-armv7.tar.gz'.encode()
    if sidecar.strip() != expected_row:
        p.error('candidate sidecar content mismatch')
    if digest(updater) != UPDATER_SHA:
        p.error('candidate updater bytes mismatch')
    with tarfile.open(fileobj=io.BytesIO(archive), mode='r:gz') as tf:
        members = tf.getnames()
        expected_members = {
            'unifi-bridge-armv7', 'install.sh', 'start.sh', 'apply_config.sh',
            'sync_nodes.sh', 'unifi-bridge.sh', 'update.sh', 'watchdog.sh',
            'unifi-bridge.asp', 'version.txt', 'bundle.json',
        }
        if set(members) != expected_members or len(members) != len(expected_members):
            p.error('candidate archive member set mismatch')
        if not all(member.isfile() and '/' not in member.name and member.name not in ('.', '..')
                   for member in tf.getmembers()):
            p.error('candidate archive contains a non-regular or unsafe member')
        if digest(tf.extractfile('update.sh').read()) != UPDATER_SHA:
            p.error('candidate archive updater bytes mismatch')
    diag = pathlib.Path(__file__).with_name('diagnose.remote.sh').read_bytes()
    args.output.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
    parent_stat = args.output.parent.stat()
    if parent_stat.st_uid != os.getuid() or parent_stat.st_mode & 0o077:
        p.error('output parent must be owned by this user and private (no group/other permissions)')
    command_path = args.output.with_suffix(args.output.suffix + '.receiver.command')
    if args.output.exists() or command_path.exists():
        p.error('output payload/receiver path already exists; choose a fresh private path')
    payload = io.BytesIO()
    with tarfile.open(fileobj=payload, mode='w:gz') as tf:
        add_bytes(tf, 'diagnose.remote.sh', diag)
        add_bytes(tf, 'candidate.tar.gz', archive)
        add_bytes(tf, 'candidate.tar.gz.sha256', sidecar)
        add_bytes(tf, 'candidate-update.sh', updater)
    data = payload.getvalue()
    args.output.write_bytes(data)
    args.output.chmod(0o600)
    command = remote_command(args.request_id, digest(data))
    command_path.write_text(command + '\n')
    command_path.chmod(0o600)
    print(f'candidate={COMMIT} archive_sha256={ARCHIVE_SHA} updater_sha256={UPDATER_SHA}')
    print(f'payload_sha256={digest(data)} payload_bytes={len(data)}')
    print(f'payload_path={args.output}')
    print(f'receiver_command_path={command_path}')
    print(f'remote_stage=/tmp/unifi-main-ramdiag-{COMMIT}-{args.request_id}')
    print(f'request_id={args.request_id}')
    print('remote_execution=not_run')

if __name__ == '__main__':
    main()
