#!/usr/bin/env python3
"""Run one foreground local command while durably retaining its result and logs."""
import argparse
from datetime import datetime, timezone
import json
import os
import pathlib
import signal
import subprocess
import sys
import time


def utc_now():
    return datetime.now(timezone.utc).isoformat(timespec="seconds")


def atomic_json(path, value):
    temporary = path.with_name(path.name + f".tmp-{os.getpid()}")
    fd = os.open(temporary, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd, "w") as stream:
        json.dump(value, stream, sort_keys=True)
        stream.write("\n")
        stream.flush()
        os.fsync(stream.fileno())
    os.replace(temporary, path)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--result", type=pathlib.Path, required=True)
    parser.add_argument("--stdout", type=pathlib.Path, required=True)
    parser.add_argument("--stderr", type=pathlib.Path, required=True)
    parser.add_argument("--stdin", type=pathlib.Path)
    parser.add_argument("--timeout-seconds", type=float, default=180.0)
    parser.add_argument("command", nargs=argparse.REMAINDER)
    args = parser.parse_args()
    command = args.command[1:] if args.command[:1] == ["--"] else args.command
    if not command or args.timeout_seconds <= 0:
        parser.error("provide a command after -- and a positive timeout")
    result_path, stdout_path, stderr_path = args.result, args.stdout, args.stderr
    parent = result_path.parent.resolve()
    if any(path.parent.resolve() != parent for path in (stdout_path, stderr_path)):
        parser.error("result and log files must share one private directory")
    try:
        st = parent.stat()
    except OSError as error:
        parser.error(f"cannot inspect result directory: {error}")
    if st.st_uid != os.getuid() or st.st_mode & 0o077:
        parser.error("result directory must be owned by this user and mode 0700 or stricter")
    if any(path.exists() or path.is_symlink() for path in (result_path, stdout_path, stderr_path)):
        parser.error("result or log path already exists")
    if args.stdin and not args.stdin.is_file():
        parser.error("stdin input is not a regular file")

    started_at = utc_now()
    start = time.monotonic()
    record = {
        "status": "running", "started_at_utc": started_at,
        "timeout_seconds": args.timeout_seconds, "client_exit_code": None,
        "elapsed_seconds": None, "timed_out": False,
    }
    atomic_json(result_path, record)
    stdin_stream = args.stdin.open("rb") if args.stdin else subprocess.DEVNULL
    for path in (stdout_path, stderr_path):
        fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        os.close(fd)
    try:
        with stdout_path.open("ab") as stdout, stderr_path.open("ab") as stderr:
            process = subprocess.Popen(command, stdin=stdin_stream, stdout=stdout,
                                       stderr=stderr, start_new_session=True)
            record["pid"] = process.pid
            atomic_json(result_path, record)
            try:
                process.wait(timeout=args.timeout_seconds)
                record["client_exit_code"] = process.returncode
                if process.returncode < 0:
                    record["status"] = "terminated_by_signal_remote_completion_unknown"
                    record["termination_signal"] = -process.returncode
                else:
                    record["status"] = "completed"
            except subprocess.TimeoutExpired:
                record["status"] = "timed_out_remote_completion_unknown"
                record["timed_out"] = True
                try:
                    os.killpg(process.pid, signal.SIGTERM)
                except ProcessLookupError:
                    pass
                try:
                    process.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    try:
                        os.killpg(process.pid, signal.SIGKILL)
                    except ProcessLookupError:
                        pass
                    process.wait()
    except OSError as error:
        record["status"] = "spawn_error"
        record["error_type"] = type(error).__name__
        record["error_errno"] = error.errno
        record["client_exit_code"] = None
    finally:
        if args.stdin:
            stdin_stream.close()
    record["completed_at_utc"] = utc_now()
    record["elapsed_seconds"] = round(time.monotonic() - start, 3)
    record["stdout_bytes"] = stdout_path.stat().st_size
    record["stderr_bytes"] = stderr_path.stat().st_size
    atomic_json(result_path, record)
    print(f"status={record['status']} client_exit_code={record['client_exit_code']} elapsed_seconds={record['elapsed_seconds']:.3f} result={result_path}")
    if record["status"] == "completed":
        return min(255, max(0, process.returncode))
    if record["status"] == "terminated_by_signal_remote_completion_unknown":
        return min(255, 128 + record["termination_signal"])
    if record["timed_out"]:
        return 124
    return 127


if __name__ == "__main__":
    raise SystemExit(main())
