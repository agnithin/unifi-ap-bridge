#!/usr/bin/env python3
"""Exercise sanitized update.sh observations and comparison classifications locally."""
import pathlib
import re
import hashlib
import subprocess
import tempfile

ROOT = pathlib.Path(__file__).resolve().parent
TEMPLATE = (ROOT / "launch.remote.template.sh").read_text()
ARCHIVE_SHA = "16300c4bc0f08f6d5bfadae8427a7928f01de1470eaf7d000c317be6d27657f1"
NORMALIZED_SIDECAR_SHA = "0e230741aefdf01fdff06eb291693299edcaeddc9ed7e45fb646593f2252116a"


def function(name, single_line=False):
    if single_line:
        pattern = rf"(?m)^{name}\(\)[^\n]*$"
    else:
        pattern = rf"(?ms)^{name}\(\) \{{\n.*?^\}}"
    match = re.search(pattern, TEMPLATE)
    if not match:
        raise ValueError(f"missing shell function {name}")
    return match.group(0)


def main():
    source = "\n".join([
        function("perm", True), function("file_state"), function("file_readable", True),
        function("compare_candidate_file"), function("compare_candidate_mode"),
        function("observe_candidate_file"), function("verify_installed_sidecar"),
    ])
    with tempfile.TemporaryDirectory(prefix="instrumented-observation-") as temporary:
        root = pathlib.Path(temporary)
        stage = root / "stage"
        stage.mkdir(mode=0o700)
        (stage / "candidate-observations.status").touch(mode=0o600)
        src, dst = root / "candidate-update", root / "installed-update"
        secret = b"synthetic-secret-like-fixture-content\n"
        src.write_bytes(secret); dst.write_bytes(secret)
        src.chmod(0o600); dst.chmod(0o600)
        unreadable = root / "unreadable-update"
        unreadable.write_bytes(secret); unreadable.chmod(0o600)
        different = root / "different-update"
        different.write_text("different synthetic bytes\n"); different.chmod(0o600)
        control = root / "observation-test.sh"
        control.write_text("#!/bin/sh\nset -eu\n")
        shell = f'''set -eu
STAGE={str(stage)!r}
TEST_UNREADABLE={str(unreadable)!r}
{source}
file_readable() {{ [ "$1" != "$TEST_UNREADABLE" ] && [ -r "$1" ]; }}
observe_candidate_file after_handoff updater {str(src)!r}
observe_candidate_file missing_case updater {str(root / "absent")!r}
observe_candidate_file unreadable_case updater "$TEST_UNREADABLE"
ln -s {str(src)!r} {str(root / "update-link")!r}
observe_candidate_file symlink_case updater {str(root / "update-link")!r}
row=0
while [ "$row" -lt 28 ]; do observe_candidate_file bounded_case updater {str(src)!r}; row=$((row + 1)); done
if observe_candidate_file overflow_case updater {str(src)!r}; then exit 41; fi
[ "$(compare_candidate_file {str(src)!r} {str(dst)!r})" = same ]
printf 'same content=same mode=%s\\n' "$(compare_candidate_mode {str(src)!r} {str(dst)!r})"
printf 'changed_content=%s\\n' "$(compare_candidate_file {str(src)!r} {str(different)!r})"
printf 'missing_source=%s\\n' "$(compare_candidate_file {str(root / "absent")!r} {str(dst)!r})"
printf 'unreadable_source=%s\\n' "$(compare_candidate_file "$TEST_UNREADABLE" {str(dst)!r})"
printf 'unreadable_destination=%s\\n' "$(compare_candidate_file {str(src)!r} "$TEST_UNREADABLE")"
printf 'missing_destination=%s\\n' "$(compare_candidate_file {str(src)!r} {str(root / "absent-destination")!r})"
chmod 644 {str(dst)!r}
printf 'mode_difference=%s\\n' "$(compare_candidate_mode {str(src)!r} {str(dst)!r})"
'''
        fakebin = root / "bin"; fakebin.mkdir()
        fake_cmp = fakebin / "cmp"
        fake_cmp.write_text("#!/bin/sh\nexit 2\n"); fake_cmp.chmod(0o700)
        shell += f'''PATH={str(fakebin)!r}:$PATH
export PATH
printf 'comparison_error=%s\\n' "$(compare_candidate_file {str(src)!r} {str(dst)!r})"
'''
        result = subprocess.run(["/bin/sh", "-eu", "-c", shell], capture_output=True, text=True, timeout=5)
        if result.returncode:
            raise RuntimeError(result.stderr or f"fixture exit {result.returncode}")
        log = (stage / "candidate-observations.status").read_text()
        if secret.decode().strip() in log:
            raise AssertionError("observation log exposed fixture file content")
        expected = {
            "phase=after_handoff file=updater state=present type=regular",
            "phase=missing_case file=updater state=missing type=missing",
            "phase=unreadable_case file=updater state=unreadable type=regular",
            "phase=symlink_case file=updater state=present type=symlink",
        }
        if not expected.issubset(set(line.rsplit(" mode=", 1)[0] for line in log.splitlines())):
            raise AssertionError("observation classifications missing")
        regular_row = next(line for line in log.splitlines() if "phase=after_handoff" in line)
        if f"bytes={len(secret)}" not in regular_row or f"sha256={hashlib.sha256(secret).hexdigest()}" not in regular_row:
            raise AssertionError("regular-file size/hash observation missing")
        if len(log.splitlines()) != 32 or "overflow_case" in log:
            raise AssertionError("observation row bound failed")
        statuses = set(result.stdout.splitlines())
        required = {
            "same content=same mode=same", "changed_content=different", "missing_source=source_missing",
            "missing_destination=destination_missing", "unreadable_source=source_unreadable",
            "unreadable_destination=destination_unreadable", "mode_difference=different",
            "comparison_error=comparison_error",
        }
        if not required.issubset(statuses):
            raise AssertionError(f"comparison classifications missing: {statuses}")
        normalized_dir = root / "sidecar-fixtures"
        normalized_dir.mkdir()
        normalized = normalized_dir / "correct.sha256"
        normalized.write_text(f"{ARCHIVE_SHA}  unifi-bridge.tar.gz\n")
        if hashlib.sha256(normalized.read_bytes()).hexdigest() != NORMALIZED_SIDECAR_SHA:
            raise AssertionError("fixture does not match pinned normalized sidecar digest")
        def sidecar_shell(path, pinned_sha):
            return f'''ARCHIVE_SHA={ARCHIVE_SHA!r}
INSTALLED_SIDECAR_SHA={pinned_sha!r}
{function("file_readable", True)}
{function("verify_installed_sidecar")}
verify_installed_sidecar {str(path)!r}
'''
        valid = subprocess.run(["/bin/sh", "-eu", "-c", sidecar_shell(normalized, NORMALIZED_SIDECAR_SHA)], capture_output=True, text=True, timeout=5)
        if valid.returncode:
            raise AssertionError("correct normalized sidecar rejected: " + valid.stderr)
        for label, row in (
            ("wrong_digest", f"{'0' * 64}  unifi-bridge.tar.gz\n"),
            ("wrong_filename", f"{ARCHIVE_SHA}  unifi-bridge-addon-candidate-armv7.tar.gz\n"),
        ):
            invalid = normalized_dir / f"{label}.sha256"
            invalid.write_text(row)
            # Pin malformed bytes so rejection exercises the row's archive
            # digest/filename constraint rather than only the file hash.
            malformed_pin = hashlib.sha256(invalid.read_bytes()).hexdigest()
            rejected = subprocess.run(["/bin/sh", "-eu", "-c", sidecar_shell(invalid, malformed_pin)],
                                      capture_output=True, text=True, timeout=5)
            if rejected.returncode == 0:
                raise AssertionError(f"normalized sidecar accepted {label}")
    print("instrumented observations, comparison classes and normalized sidecar fixtures: PASS")


if __name__ == "__main__":
    main()
