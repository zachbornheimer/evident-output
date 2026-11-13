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
import time
import uuid
from pathlib import Path

SCRIPT_DIR = Path(__file__).resolve().parent
if str(SCRIPT_DIR) not in sys.path:
    sys.path.insert(0, str(SCRIPT_DIR))

import testevo_render
import testevo_session

DEFAULT_COLS = 120
DEFAULT_ROWS = 40
KILL_AFTER_SECONDS = "0.2"
SESSION_PREFIX = "test-evo-"
USAGE_EXIT = 2
CHILD_EXIT_NAME = "child_exit"
RUNNER_NAME = "_run.sh"
FRAME_PREFIX = "frame-"


def usage_text() -> str:
    temp = tempfile.gettempdir()
    return f"""usage: python3 ./scripts/test-evo.py --start-dir DIR --timeout SECONDS --screenshot-interval-ms MS [--output-frames DIR] [--cols N] [--rows N] -- CMD [ARGS...]

Capture timed PNG/TXT frames of a command running in a tmux PTY.

Required:
  --start-dir DIR                 Pane cwd (replaces --start-in; --start-in is a hidden alias)
  --timeout SECONDS               GNU timeout duration for the inner command (int or float)
  --screenshot-interval-ms MS     Capture period in milliseconds (positive int)
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

    parser = argparse.ArgumentParser(add_help=False)
    parser.add_argument("--start-dir", "--start-in", dest="start_dir", default=None)
    parser.add_argument("--timeout", default=None)
    parser.add_argument("--screenshot-interval-ms", default=None)
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
    if args.timeout is None:
        die_usage("error: --timeout is required")
    if args.screenshot_interval_ms is None:
        die_usage("error: --screenshot-interval-ms is required")

    try:
        timeout = float(args.timeout)
    except ValueError:
        die_usage("error: --timeout must be a number (seconds)")
    if timeout <= 0:
        die_usage("error: --timeout must be positive")

    try:
        interval_ms = int(args.screenshot_interval_ms)
    except ValueError:
        die_usage("error: --screenshot-interval-ms must be a positive int")
    if interval_ms <= 0:
        die_usage("error: --screenshot-interval-ms must be a positive int")

    if args.cols <= 0 or args.rows <= 0:
        die_usage("error: --cols and --rows must be positive")

    start_dir = Path(args.start_dir).expanduser()
    if not start_dir.is_dir():
        die_usage(f"error: --start-dir is not a directory: {start_dir}")

    args.start_dir = start_dir.resolve()
    args.timeout = timeout
    args.screenshot_interval_ms = interval_ms
    args.cmd = cmd
    return args


def resolve_frames_dir(flag_value: str | None) -> Path:
    if flag_value:
        path = Path(flag_value).expanduser().resolve()
    else:
        path = Path(tempfile.gettempdir()) / f"test-evo-screenshots-{uuid.uuid4()}"
    path.mkdir(parents=True, exist_ok=True)
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
    body = f"""#!/usr/bin/env bash
{exports}
{shlex.quote(timeout_bin)} --foreground --kill-after={KILL_AFTER_SECONDS} {shlex.quote(str(timeout_secs))} {cmd_joined}
ec=$?
printf '%s\\n' "$ec" > {shlex.quote(str(child_exit_path))}
exit 0
"""
    runner.write_text(body, encoding="utf-8")
    runner.chmod(0o755)


def write_frame(frames_dir: Path, index: int, text: str, cols: int, rows: int) -> None:
    stem = f"{FRAME_PREFIX}{index:05d}"
    (frames_dir / f"{stem}.txt").write_text(text, encoding="utf-8")
    testevo_render.render_ansi_to_png(
        text, frames_dir / f"{stem}.png", cols=cols, rows=rows
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
        "_TEST_EVO_TIMEOUT": str(args.timeout),
        "_TEST_EVO_SCREENSHOT_INTERVAL_MS": str(args.screenshot_interval_ms),
        "_TEST_EVO_OUTPUT_FRAMES": str(frames_dir),
        "_TEST_EVO_CMD": shlex.join(args.cmd),
        "_TEST_EVO_COLS": str(args.cols),
        "_TEST_EVO_ROWS": str(args.rows),
    }

    meta = {
        "start_dir": str(args.start_dir),
        "timeout": args.timeout,
        "interval_ms": args.screenshot_interval_ms,
        "cmd": args.cmd,
        "cols": args.cols,
        "rows": args.rows,
        "session": session,
        "output_frames": str(frames_dir),
        "child_exit": None,
    }
    write_meta(frames_dir / "meta.json", meta)
    write_runner(
        runner_path,
        env=env,
        timeout_bin=timeout_bin,
        timeout_secs=args.timeout,
        cmd=args.cmd,
        child_exit_path=child_exit_path,
    )

    try:
        testevo_session.start_capture_session(
            session=session,
            cols=args.cols,
            rows=args.rows,
            start_dir=str(args.start_dir),
            path_env=os.environ.get("PATH", ""),
            runner_path=str(runner_path),
        )
    except (subprocess.CalledProcessError, FileNotFoundError) as err:
        testevo_session.kill_session(session)
        print(f"error: failed to start tmux session: {err}", file=sys.stderr)
        return 1

    command_pid = testevo_session.pane_pid(session)
    interval_s = args.screenshot_interval_ms / 1000.0
    deadline = time.monotonic() + float(args.timeout) + float(KILL_AFTER_SECONDS) + 5.0
    frame_index = 0
    harness_ok = True

    try:
        while True:
            if not testevo_session.session_exists(session):
                break
            try:
                text = testevo_session.capture_pane(session)
                write_frame(frames_dir, frame_index, text, args.cols, args.rows)
                frame_index += 1
            except (subprocess.CalledProcessError, OSError) as err:
                print(f"error: capture/render failed: {err}", file=sys.stderr)
                harness_ok = False
                break

            if testevo_session.pane_is_dead(session):
                try:
                    text = testevo_session.capture_pane(session)
                    write_frame(frames_dir, frame_index, text, args.cols, args.rows)
                    frame_index += 1
                except (subprocess.CalledProcessError, OSError):
                    pass
                break

            if time.monotonic() > deadline:
                break
            time.sleep(interval_s)
    finally:
        meta["child_exit"] = read_child_exit(child_exit_path)
        meta["frame_count"] = frame_index
        write_meta(frames_dir / "meta.json", meta)
        testevo_session.kill_session(session, command_pid=command_pid)

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
