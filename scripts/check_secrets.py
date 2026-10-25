#!/usr/bin/env python3
"""Reject literal X-API-Key credentials without printing their values.

This narrow guard complements GitGuardian; it is not a general secret scanner.
"""
from pathlib import Path
import re
import subprocess
import sys

ROOT=Path(__file__).resolve().parent.parent
HEADER=re.compile(r"(?i)x-api-key[\"']?\s*:\s*[\"']?\s*([A-Za-z0-9_-]{20,})")
EXTENSIONS={".md",".sh",".go",".json",".yml",".yaml",".asp",".py",".js",".txt"}
def findings(text):
    normalized=text.replace('\\"','"').replace("\\'", "'")
    return [normalized.count("\n",0,m.start())+1 for m in HEADER.finditer(normalized)]
def main():
    try:
        names=subprocess.check_output(["git","ls-files","--cached","--others","--exclude-standard","-z"],cwd=ROOT,stderr=subprocess.DEVNULL).decode().split("\0")
        paths=[ROOT/n for n in names if n]
    except subprocess.CalledProcessError:
        paths=[p for p in ROOT.rglob("*") if p.is_file() and ".git" not in p.parts]
    failed=False
    for path in paths:
        if path.suffix not in EXTENSIONS:continue
        try:text=path.read_text()
        except (UnicodeError,OSError):continue
        for line in findings(text):
            print(f"{path.relative_to(ROOT)}:{line}: literal API-key header credential detected (value redacted)",file=sys.stderr);failed=True
    if not failed:print("Literal API-key header guard passed")
    return int(failed)
if __name__=="__main__":sys.exit(main())
