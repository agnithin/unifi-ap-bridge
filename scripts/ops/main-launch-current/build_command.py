#!/usr/bin/env python3
import pathlib
import re
import shlex
import sys

if len(sys.argv) == 4 and sys.argv[1] == '--handoff':
    handoff_dir, expected_sha = sys.argv[2:4]
    if not re.fullmatch(r'[0-9a-f]{64}', expected_sha):
        raise SystemExit('handoff checksum must be lowercase SHA-256')
    directory = shlex.quote(handoff_dir)
    checksum = shlex.quote(expected_sha)
    sys.stdout.write(f'''set -eu
umask 077
handoff_dir={directory}
mkdir -m 700 "$handoff_dir" || {{ echo 'handoff refused: private directory already exists or cannot be created' >&2; exit 72; }}
handoff="$handoff_dir/script.sh"
cat > "$handoff"
chmod 700 "$handoff"
actual=$(openssl dgst -sha256 "$handoff" | awk '{{print $NF}}')
[ "$actual" = {checksum} ] || {{ echo 'handoff checksum mismatch; retained for diagnosis' >&2; exit 73; }}
sh -n "$handoff" || {{ echo 'handoff syntax failure; retained for diagnosis' >&2; exit 74; }}
if /bin/sh "$handoff"; then rc=0; else rc=$?; fi
if [ "$rc" -eq 0 ]; then rm -f "$handoff" && rmdir "$handoff_dir"; fi
exit "$rc"
''')
else:
    template_path, script_path = map(pathlib.Path, sys.argv[1:3])
    template = template_path.read_text()
    script = script_path.read_text()
    marker = '@SCRIPT_BODY@'
    if template.count(marker) != 1:
        raise SystemExit('expected exactly one script-body marker')
    quoted = "'" + script.replace("'", "'\\''") + "'"
    sys.stdout.write(template.replace(marker, quoted))
