#!/usr/bin/env python3
"""TTY capture harness: photograph a live CLI as timed PNG + TXT frames via tmux."""

from __future__ import annotations

import argparse
import json
import os
import shlex
import shutil
import subprocess
import sys
import tempfile
import threading
import time
import uuid
from datetime import datetime
from pathlib import Path

import testevo_frames
import testevo_render
import testevo_session

DEFAULT_COLS = 120
DEFAULT_ROWS = 40
KILL_AFTER_SECONDS = "0.2"
SESSION_PREFIX = "test-evo-"
USAGE_EXIT = 2
CHILD_EXIT_NAME = "child_exit"
RUNNER_NAME = "_run.sh"
STALE_GLOBS = ("frame-*", "timed-*", "mandatory-*")


def usage_text() -> str:
    temp = tempfile.gettempdir()
    return f"""usage: python3 ./scripts/test-evo.py --start-dir DIR \\
  (--timeout-ms MS | --timeout-s S) \\
  (--screenshot-interval-ms MS | --screenshot-interval-s S) \\
  [--output-frames DIR] [--cols N] [--rows N] -- CMD [ARGS...]

Capture timed PNG/TXT frames of a command running in a tmux PTY.

Always also takes five mandatory frames: before command, immediately after
submit, then 100/200/300ms later (spinner-position checks on the last two).
After capture, prints [OK]/[FAIL]/[SKIP] assertions on mandatory-2..5.

Required:
  --start-dir DIR                 Pane cwd (replaces --start-in; --start-in is a hidden alias)
  --timeout-ms MS | --timeout-s S
                                  Inner-command duration (exactly one)
  --screenshot-interval-ms MS | --screenshot-interval-s S
                                  Capture period (exactly one)
  -- CMD [ARGS...]                Command run inside the pane (everything after --)

Optional:
  --output-frames DIR             Write frames here (default: {temp}/test-evo-screenshots-<uuid>/)
  --cols N                        Pane width (default: {DEFAULT_COLS})
  --rows N                        Pane height (default: {DEFAULT_ROWS})
"""


def die_usage(message: str | None = None) -> None:
    if message:
        print(message, file=sys.stderr)
    print(usage_text(), file=sys.stderr, end="")
    raise SystemExit(USAGE_EXIT)


def parse_args(argv: list[str]) -> argparse.Namespace:
    if "--" not in argv:
        die_usage("error: missing '--' before command")
    dash = argv.index("--")
    pre = argv[:dash]
    cmd = argv[dash + 1 :]
    if not cmd:
        die_usage("error: empty command after '--'")

    parser = argparse.ArgumentParser(add_help=False, allow_abbrev=False)
    parser.add_argument("--start-dir", "--start-in", dest="start_dir", default=None)
    parser.add_argument("--timeout-ms", default=None)
    parser.add_argument("--timeout-s", default=None)
    parser.add_argument("--screenshot-interval-ms", default=None)
    parser.add_argument("--screenshot-interval-s", default=None)
    parser.add_argument("--output-frames", default=None)
    parser.add_argument("--cols", type=int, default=DEFAULT_COLS)
    parser.add_argument("--rows", type=int, default=DEFAULT_ROWS)
    parser.add_argument("-h", "--help", action="store_true")

    try:
        args, unknown = parser.parse_known_args(pre)
    except SystemExit:
        die_usage()

    if args.help:
        print(usage_text(), end="")
        raise SystemExit(0)
    if unknown:
        die_usage(f"error: unrecognized arguments: {' '.join(unknown)}")
    if args.start_dir is None:
        die_usage("error: --start-dir is required")

    timeout_s, timeout_env = parse_duration_pair(
        args.timeout_ms,
        args.timeout_s,
        "--timeout-ms",
        "--timeout-s",
    )
    interval_s, interval_env = parse_duration_pair(
        args.screenshot_interval_ms,
        args.screenshot_interval_s,
        "--screenshot-interval-ms",
        "--screenshot-interval-s",
    )

    if args.cols <= 0 or args.rows <= 0:
        die_usage("error: --cols and --rows must be positive")

    start_dir = Path(args.start_dir).expanduser()
    if not start_dir.is_dir():
        die_usage(f"error: --start-dir is not a directory: {start_dir}")

    args.start_dir = start_dir.resolve()
    args.timeout_s = timeout_s
    args.interval_s = interval_s
    args.duration_env = {**timeout_env, **interval_env}
    args.cmd = cmd
    return args


def parse_duration_pair(
    ms_raw: str | None,
    s_raw: str | None,
    flag_ms: str,
    flag_s: str,
) -> tuple[float, dict[str, str]]:
    if ms_raw is not None and s_raw is not None:
        die_usage(f"error: pass only one of {flag_ms} or {flag_s}")
    if ms_raw is None and s_raw is None:
        die_usage(f"error: one of {flag_ms} or {flag_s} is required")
    if ms_raw is not None:
        value = parse_positive_number(ms_raw, flag_ms)
        return value / 1000.0, {env_name(flag_ms): str(ms_raw)}
    value = parse_positive_number(s_raw or "", flag_s)
    return value, {env_name(flag_s): str(s_raw)}


def parse_positive_number(raw: str, flag: str) -> float:
    try:
        value = float(raw)
    except ValueError:
        die_usage(f"error: {flag} must be a number")
    if value <= 0:
        die_usage(f"error: {flag} must be positive")
    return value


def env_name(flag: str) -> str:
    return "_TEST_EVO_" + flag.lstrip("-").replace("-", "_").upper()


def capture_stamp(when: datetime | None = None) -> str:
    now = (when or datetime.now()).astimezone()
    hundredths = now.microsecond // 10000
    zone = now.strftime("%Z") or now.tzname() or "local"
    return now.strftime("%Y-%m-%d %H:%M:%S") + f".{hundredths:02d} {zone}"


def resolve_frames_dir(flag_value: str | None) -> Path:
    if flag_value:
        path = Path(flag_value).expanduser().resolve()
    else:
        path = Path(tempfile.gettempdir()) / f"test-evo-screenshots-{uuid.uuid4()}"
    path.mkdir(parents=True, exist_ok=True)
    for pattern in STALE_GLOBS:
        for stale in path.glob(pattern):
            stale.unlink()
    return path


def find_timeout_bin() -> str:
    for name in ("timeout", "gtimeout"):
        found = shutil.which(name)
        if found:
            return found
    raise RuntimeError("GNU timeout not found on PATH (tried timeout, gtimeout)")


def write_meta(path: Path, payload: dict) -> None:
    path.write_text(json.dumps(payload, indent=2, sort_keys=True) + "\n", encoding="utf-8")


def write_runner(
    runner: Path,
    *,
    env: dict[str, str],
    timeout_bin: str,
    timeout_secs: float,
    cmd: list[str],
    child_exit_path: Path,
) -> None:
    exports = "\n".join(f"export {k}={shlex.quote(v)}" for k, v in env.items())
    cmd_joined = shlex.join(cmd)
    # GNU timeout treats args after DURATION as COMMAND; a bare "--" becomes the command.
    timeout_cmd = (
        f"{shlex.quote(timeout_bin)} --foreground "
        f"--kill-after={KILL_AFTER_SECONDS} {shlex.quote(str(timeout_secs))} "
        f"{cmd_joined}"
    )
    body = f"""#!/usr/bin/env bash
{exports}
{timeout_cmd}
ec=$?
printf '%s\\n' "$ec" > {shlex.quote(str(child_exit_path))}
exit 0
"""
    runner.write_text(body, encoding="utf-8")
    runner.chmod(0o755)


def write_txt_frame(frames_dir: Path, stem: str, text: str) -> Path:
    dest = frames_dir / f"{stem}.txt"
    dest.write_text(text, encoding="utf-8")
    return dest


def write_final_frames(
    frames_dir: Path,
    records: list[testevo_frames.FrameRecord],
    cols: int,
    rows: int,
) -> dict[str, int]:
    widths = testevo_frames.widths_for(records)
    for record in records:
        stem = record.stem(widths[record.kind])
        write_txt_frame(frames_dir, stem, record.text)
        testevo_render.render_ansi_to_png(
            record.text,
            frames_dir / f"{stem}.png",
            cols=cols,
            rows=rows,
            captured_at=record.captured_at,
            name=stem,
            reason=record.reason,
        )
    return widths


def capture_record(session: str, kind: str, index: int, reason: str) -> testevo_frames.FrameRecord:
    text = testevo_session.capture_pane(session)
    return testevo_frames.FrameRecord(
        kind=kind,
        index=index,
        captured_at=capture_stamp(),
        reason=reason,
        text=text,
    )


def read_child_exit(path: Path) -> int | None:
    if not path.is_file():
        return None
    raw = path.read_text(encoding="utf-8").strip()
    if not raw:
        return None
    try:
        return int(raw)
    except ValueError:
        return None


def run_harness(args: argparse.Namespace) -> int:
    frames_dir = resolve_frames_dir(args.output_frames)
    print(str(frames_dir), flush=True)

    session = f"{SESSION_PREFIX}{uuid.uuid4().hex[:12]}"
    child_exit_path = frames_dir / CHILD_EXIT_NAME
    runner_path = frames_dir / RUNNER_NAME
    timeout_bin = find_timeout_bin()

    env = {
        "_TEST_EVO_STARTING_DIR": str(args.start_dir),
        "_TEST_EVO_OUTPUT_FRAMES": str(frames_dir),
        "_TEST_EVO_CMD": shlex.join(args.cmd),
        "_TEST_EVO_COLS": str(args.cols),
        "_TEST_EVO_ROWS": str(args.rows),
        **args.duration_env,
    }

    meta: dict = {
        "start_dir": str(args.start_dir),
        "timeout_s": args.timeout_s,
        "interval_s": args.interval_s,
        "cmd": args.cmd,
        "cols": args.cols,
        "rows": args.rows,
        "session": session,
        "output_frames": str(frames_dir),
        "child_exit": None,
        "frames": [],
    }
    write_meta(frames_dir / "meta.json", meta)
    write_runner(
        runner_path,
        env=env,
        timeout_bin=timeout_bin,
        timeout_secs=args.timeout_s,
        cmd=args.cmd,
        child_exit_path=child_exit_path,
    )

    records: list[testevo_frames.FrameRecord] = []
    records_lock = threading.Lock()
    capture_lock = threading.Lock()
    stop_timed = threading.Event()
    command_pid: int | None = None
    harness_ok = True

    def snap(kind: str, index: int, reason: str) -> testevo_frames.FrameRecord:
        with capture_lock:
            record = capture_record(session, kind, index, reason)
        with records_lock:
            records.append(record)
        return record

    try:
        testevo_session.create_idle_session(
            session=session,
            cols=args.cols,
            rows=args.rows,
            start_dir=str(args.start_dir),
            path_env=os.environ.get("PATH", ""),
        )
    except (subprocess.CalledProcessError, FileNotFoundError) as err:
        testevo_session.kill_session(session)
        print(f"error: failed to start tmux session: {err}", file=sys.stderr)
        return 1

    timed_thread: threading.Thread | None = None
    try:
        snap(
            testevo_frames.KIND_MANDATORY,
            testevo_frames.MANDATORY_BEFORE_COMMAND,
            testevo_frames.MANDATORY_BEFORE_REASON,
        )
        testevo_session.submit_runner(session, str(runner_path))
        submit_t = time.monotonic()
        interval_s = float(args.interval_s)
        deadline = submit_t + float(args.timeout_s) + float(KILL_AFTER_SECONDS) + 5.0
        timed_reason = testevo_frames.timed_reason(interval_s)

        def run_timed() -> None:
            index = 1
            next_t = submit_t
            while not stop_timed.is_set():
                if child_exit_path.is_file() or time.monotonic() > deadline:
                    return
                wait = next_t - time.monotonic()
                if wait > 0 and stop_timed.wait(wait):
                    return
                if child_exit_path.is_file() or time.monotonic() > deadline:
                    return
                try:
                    snap(testevo_frames.KIND_TIMED, index, timed_reason)
                except (subprocess.CalledProcessError, OSError):
                    return
                index += 1
                next_t += interval_s
                now = time.monotonic()
                if next_t < now:
                    skip = int((now - next_t) / interval_s) + 1
                    next_t += skip * interval_s

        timed_thread = threading.Thread(target=run_timed, name="test-evo-timed", daemon=True)
        timed_thread.start()

        due = submit_t
        for index, reason in testevo_frames.MANDATORY_AFTER_SUBMIT:
            wait = due - time.monotonic()
            if wait > 0:
                time.sleep(wait)
            snap(testevo_frames.KIND_MANDATORY, index, reason)
            due += testevo_frames.MANDATORY_GAP_S

        while not child_exit_path.is_file() and time.monotonic() <= deadline:
            time.sleep(0.05)
    finally:
        stop_timed.set()
        if timed_thread is not None:
            timed_thread.join(timeout=2.0)
        widths = testevo_frames.widths_for(records)
        meta["child_exit"] = read_child_exit(child_exit_path)
        meta["timed_count"] = sum(1 for r in records if r.kind == testevo_frames.KIND_TIMED)
        meta["mandatory_count"] = sum(1 for r in records if r.kind == testevo_frames.KIND_MANDATORY)
        meta["frames"] = [
            {
                "kind": r.kind,
                "index": r.index,
                "name": r.stem(widths[r.kind]),
                "captured_at": r.captured_at,
                "reason": r.reason,
            }
            for r in records
        ]
        checks = testevo_frames.evaluate_mandatory_assertions(records)
        meta["assertions"] = [
            {"name": c.name, "status": c.status, "detail": c.detail} for c in checks
        ]
        write_meta(frames_dir / "meta.json", meta)
        (frames_dir / "assertions.txt").write_text(
            "\n".join(c.line() for c in checks) + "\n",
            encoding="utf-8",
        )
        for check in checks:
            print(check.line(), flush=True)
        if testevo_frames.assertions_failed(checks):
            harness_ok = False
        if command_pid is None:
            command_pid = testevo_session.pane_pid(session)
        testevo_session.kill_session(session, command_pid=command_pid)
        try:
            write_final_frames(frames_dir, records, args.cols, args.rows)
        except OSError as err:
            print(f"error: png render failed: {err}", file=sys.stderr)
            harness_ok = False

    return 0 if harness_ok else 1


def main(argv: list[str] | None = None) -> int:
    args = parse_args(list(sys.argv[1:] if argv is None else argv))
    try:
        return run_harness(args)
    except RuntimeError as err:
        print(f"error: {err}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
