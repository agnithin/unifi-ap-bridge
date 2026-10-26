#!/usr/bin/env python3
"""Run pinned-updater lifecycle regressions in isolated local firmware fixtures."""
import argparse
import hashlib
import os
import pathlib
import subprocess
import sys
import tarfile
import tempfile

COMMIT = "1f22343206c80bda01d576dfdb813e4a911e4568"
ARCHIVE_SHA = "16300c4bc0f08f6d5bfadae8427a7928f01de1470eaf7d000c317be6d27657f1"
SIDECAR_SHA = "113a1ffc283a5a27ba1594736d827088aacab9e7936478234e0c775677eaf48f"
UPDATER_SHA = "2f2dc19723cf442a2ff5ea7ebd9e8b10b56b6a0c24ab2441eca602c14407547a"
MEMBERS = {
    "unifi-bridge-armv7", "install.sh", "start.sh", "apply_config.sh",
    "sync_nodes.sh", "unifi-bridge.sh", "update.sh", "watchdog.sh",
    "unifi-bridge.asp", "version.txt", "bundle.json",
}
ROOT = pathlib.Path(__file__).resolve().parents[4]
TEST = ROOT / "scripts/test_update_apply.py"


def digest(data):
    return hashlib.sha256(data).hexdigest()


def validate(archive_path, sidecar_path):
    archive = archive_path.read_bytes()
    sidecar = sidecar_path.read_bytes()
    if digest(archive) != ARCHIVE_SHA:
        raise ValueError("pinned archive hash mismatch")
    if digest(sidecar) != SIDECAR_SHA:
        raise ValueError("pinned sidecar hash mismatch")
    expected_row = f"{ARCHIVE_SHA}  unifi-bridge-addon-{COMMIT}-armv7.tar.gz".encode()
    if sidecar.strip() != expected_row:
        raise ValueError("pinned sidecar content mismatch")
    with tarfile.open(fileobj=__import__("io").BytesIO(archive), mode="r:gz") as tf:
        entries = tf.getmembers()
        names = [entry.name for entry in entries]
        if set(names) != MEMBERS or len(names) != len(MEMBERS):
            raise ValueError("candidate member set mismatch")
        if not all(entry.isfile() and "/" not in entry.name for entry in entries):
            raise ValueError("candidate contains non-regular or unsafe member")
        updater = tf.extractfile("update.sh").read()
        if digest(updater) != UPDATER_SHA:
            raise ValueError("pinned updater member hash mismatch")
        return archive, sidecar, updater


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--archive", type=pathlib.Path, required=True)
    parser.add_argument("--sidecar", type=pathlib.Path, required=True)
    args = parser.parse_args()
    try:
        archive, sidecar, updater = validate(args.archive, args.sidecar)
    except (OSError, ValueError, tarfile.TarError) as error:
        parser.error(str(error))

    with tempfile.TemporaryDirectory(prefix="pinned-updater-lifecycle-") as temporary:
        private = pathlib.Path(temporary)
        os.chmod(private, 0o700)
        updater_path = private / "candidate-update.sh"
        updater_path.write_bytes(updater)
        updater_path.chmod(0o600)
        env = dict(os.environ,
                   UNIFI_BRIDGE_CANDIDATE_UPDATER=str(updater_path),
                   UNIFI_BRIDGE_CANDIDATE_ARCHIVE=str(args.archive.resolve()))
        cases = [
            "ApplyFailureTests.test_successful_candidate_update_traces_update_script_through_postflight",
            "ApplyFailureTests.test_apply_exact_missing_staged_update_script_rolls_back_mixed_prior",
            "ApplyFailureTests.test_apply_exact_missing_staged_asset_fails_before_transaction",
        ]
        result = subprocess.run([sys.executable, str(TEST), *cases], env=env,
                                capture_output=True, text=True, timeout=120)
    print(f"candidate={COMMIT} archive_sha256={digest(archive)} sidecar_sha256={digest(sidecar)} updater_sha256={digest(updater)}")
    print("fixture_runtime=synthetic executable; pinned ARMv7 candidate binary is validated as data because this host cannot execute ARMv7")
    print("cases=" + ",".join(case.rsplit(".", 1)[-1] for case in cases))
    print(f"test_exit_code={result.returncode}")
    if result.stdout:
        print(result.stdout, end="" if result.stdout.endswith("\n") else "\n")
    if result.stderr:
        print(result.stderr, file=sys.stderr, end="" if result.stderr.endswith("\n") else "\n")
    return result.returncode


if __name__ == "__main__":
    raise SystemExit(main())
