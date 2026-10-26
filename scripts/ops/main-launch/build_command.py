#!/usr/bin/env python3
import pathlib
import sys

template_path, script_path = map(pathlib.Path, sys.argv[1:3])
template = template_path.read_text()
script = script_path.read_text()
marker = '@SCRIPT_BODY@'
if template.count(marker) != 1:
    raise SystemExit('expected exactly one script-body marker')
quoted = "'" + script.replace("'", "'\\''") + "'"
sys.stdout.write(template.replace(marker, quoted))
