#!/usr/bin/env python3
"""Prove the live frame keeps changing under a real pty, from the pty byte stream.

Two scenarios, both bounded by --max-gap-ms (default 100):

- default: a quiet Writer-backed Task (examples/quiet-writer), checked as below;
- --tasks N: one Group of N trivial Tasks (examples/many-tasks); after the first
  frame, no gap between changed frames may reach the bound, and every gap over
  it is printed.

Builds the example and runs it under a real pty (no tmux). The reader
timestamps every read of the pty master with a monotonic clock, rebuilds the
live frames from the byte stream (ptystream_frames), and checks the quiet
interval, from the last Writer evidence line to the settle:

- every gap between frames whose owner-row spinner glyph differs is under the bound;
- no frame stays unchanged for the bound or more;
- at least MIN_QUIET_CHANGES changed frames arrive;
- the owner row stays named, all evidence lines stay on screen, no fake progress.

With --compare-plain, the frames must also settle forward only (a settled row
never reverts, a completion count never falls), and the last frame plus the exit
code must equal the Plain rows and exit code of an identical run with
EVO_OUTPUT=plain (ptystream_truth).

A reader that was itself descheduled for the bound or more cannot prove the
bound, so that attempt is retried (up to ATTEMPTS) and never passes.
"""

from __future__ import annotations

import argparse
import fcntl
import itertools
import os
import pty
import re
import select
import signal
import struct
import subprocess
import sys
import tempfile
import termios
import time
from collections.abc import Callable
from dataclasses import dataclass
from pathlib import Path

import ptystream_frames as stream
import ptystream_truth as truth

ROOT = Path(__file__).resolve().parent.parent
QUIET_EXAMPLE = "./examples/quiet-writer"
MANY_EXAMPLE = "./examples/many-tasks"
OWNER_ROW = "build"
EVIDENCE_LINES = 6
EVIDENCE = tuple(
    f"compile unit {n} of {EVIDENCE_LINES}" for n in range(1, EVIDENCE_LINES + 1)
)
QUIET_MS = 900
DEFAULT_MAX_GAP_MS = 100.0
MIN_QUIET_CHANGES = 6
MIN_MANY_CHANGES = 6
MANY_HOLD_MS = 5
ATTEMPTS = 3
QUIET_TIMEOUT_S = 10
MANY_TIMEOUT_S = 60
READ_POLL_S = 0.002
READ_SIZE = 65536
PTY_ROWS, PTY_COLS = 24, 100
TERM_VALUE = "xterm-256color"
PLAIN_OUTPUT = "plain"
FAKE_PROGRESS_RE = re.compile(r"\d+\s*%|[█░▓▒■□]")


class VerifyError(Exception):
    """The stream proves the requirement is not met."""


class Unprovable(Exception):
    """The reader was too slow to prove or disprove the bound."""


@dataclass(frozen=True)
class Scenario:
    """One example run under the pty and the check its frames must pass."""

    example: str
    args: tuple[str, ...]
    timeout_s: int
    verify: Callable[[list[stream.Frame], float], dict[str, float]]


def build_example(example: str, workdir: Path) -> Path:
    binary = workdir / Path(example).name
    subprocess.run(
        ["go", "build", "-buildvcs=false", "-o", str(binary), example],
        cwd=ROOT,
        check=True,
    )
    return binary


def run_under_pty(
    binary: Path, args: tuple[str, ...], timeout_s: int
) -> tuple[list[stream.Chunk], int, float]:
    """Return the timestamped reads, the child's exit status, and the reader's worst loop gap in ms."""
    pid, master = pty.fork()
    if pid == 0:
        fcntl.ioctl(
            0, termios.TIOCSWINSZ, struct.pack("HHHH", PTY_ROWS, PTY_COLS, 0, 0)
        )
        os.environ["TERM"] = TERM_VALUE
        os.execv(str(binary), [str(binary), *args])
    chunks: list[stream.Chunk] = []
    deadline = time.monotonic() + timeout_s
    last_loop = time.monotonic_ns()
    worst_loop_ns = 0
    try:
        while time.monotonic() < deadline:
            ready, _, _ = select.select([master], [], [], READ_POLL_S)
            now = time.monotonic_ns()
            worst_loop_ns = max(worst_loop_ns, now - last_loop)
            last_loop = now
            if not ready:
                continue
            try:
                data = os.read(master, READ_SIZE)
            except OSError:  # EIO: the child closed the pty
                break
            if not data:
                break
            chunks.append(stream.Chunk(at_ns=time.monotonic_ns(), data=data))
        else:
            os.kill(pid, signal.SIGKILL)
    finally:
        os.close(master)
    _, status = os.waitpid(pid, 0)
    return chunks, os.waitstatus_to_exitcode(status), worst_loop_ns / stream.NS_PER_MS


def run_plain(
    binary: Path, args: tuple[str, ...], timeout_s: int
) -> tuple[list[str], int]:
    """Return the stderr rows and exit status of the same run with output forced to plain."""
    # The scenario's own exit status is data to compare, not a failure to raise.
    done = subprocess.run(
        [str(binary), *args],
        check=False,
        env=os.environ | {"EVO_OUTPUT": PLAIN_OUTPUT},
        stdin=subprocess.DEVNULL,
        capture_output=True,
        text=True,
        timeout=timeout_s,
    )
    return done.stderr.splitlines(), done.returncode


def verify_truth(
    frames: list[stream.Frame], exit_code: int, binary: Path, scenario: Scenario
) -> None:
    """Frames settle forward, and the final screen is the Plain run's rows."""
    plain_rows, plain_exit = run_plain(binary, scenario.args, scenario.timeout_s)
    try:
        truth.check_monotonic(frames)
        truth.check_final_matches_plain(frames[-1], exit_code, plain_rows, plain_exit)
    except truth.TruthError as err:
        raise VerifyError(str(err)) from err


def ms(later_ns: int, earlier_ns: int) -> float:
    return (later_ns - earlier_ns) / stream.NS_PER_MS


def quiet_window(frames: list[stream.Frame]) -> list[stream.Frame]:
    """Frames from the last evidence line through the first non-running boundary."""
    start = next(
        (i for i, f in enumerate(frames) if EVIDENCE[-1] in "\n".join(f.rows)), None
    )
    if start is None:
        raise VerifyError("no frame shows the last Writer evidence line")
    end = next(
        (
            i
            for i in range(start, len(frames))
            if frames[i].owner_glyph(OWNER_ROW) is None
        ),
        len(frames) - 1,
    )
    return frames[start : end + 1]


def changed_frames(window: list[stream.Frame]) -> list[stream.Frame]:
    """First frame of each run of identical rows."""
    kept = [window[0]]
    for frame in window[1:]:
        if frame.rows != kept[-1].rows:
            kept.append(frame)
    return kept


def stalest_run_ms(window: list[stream.Frame]) -> float:
    """Longest first-to-last arrival span of frames with identical rows."""
    worst, first = 0.0, window[0]
    for prev, cur in itertools.pairwise(window):
        if cur.rows != prev.rows:
            first = cur
        worst = max(worst, ms(cur.at_ns, first.at_ns))
    return worst


def check_quiet_frames(window: list[stream.Frame]) -> None:
    for frame in window:
        if frame.owner_glyph(OWNER_ROW) is None:
            continue  # the settle boundary
        text = "\n".join(frame.rows)
        missing = [line for line in EVIDENCE if line not in text]
        if missing:
            raise VerifyError(f"quiet frame lost Writer evidence {missing[0]!r}")
        if FAKE_PROGRESS_RE.search(text):
            raise VerifyError(
                f"quiet frame shows progress the Task never reported: {text!r}"
            )


def describe_gaps(gaps: list[tuple[float, float]], max_gap_ms: float) -> str:
    listed = ", ".join(f"{gap:.0f}ms at +{start:.0f}ms" for start, gap in gaps)
    return f"{len(gaps)} frame gap(s) of {max_gap_ms:g}ms or more: {listed}"


def verify_quiet(frames: list[stream.Frame], max_gap_ms: float) -> dict[str, float]:
    window = quiet_window(frames)
    if len(window) < 2:
        raise VerifyError("the quiet interval holds one frame")
    check_quiet_frames(window)
    changes = changed_frames(window)
    quiet_changes = [f for f in changes[1:] if f.owner_glyph(OWNER_ROW) is not None]
    if len(quiet_changes) < MIN_QUIET_CHANGES:
        raise VerifyError(
            f"{len(quiet_changes)} changed frames while quiet, want at least {MIN_QUIET_CHANGES}"
        )
    gaps = stream.gaps_over(changes, max_gap_ms)
    if gaps:
        raise VerifyError(describe_gaps(gaps, max_gap_ms))
    stale = stalest_run_ms(window)
    if stale >= max_gap_ms:
        raise VerifyError(
            f"a frame stayed unchanged {stale:.0f}ms, want <{max_gap_ms:g}ms"
        )
    max_gap = max(
        ms(cur.at_ns, prev.at_ns) for prev, cur in itertools.pairwise(changes)
    )
    return {"max_gap_ms": round(max_gap, 1), "changed_frames": len(quiet_changes)}


def verify_many(frames: list[stream.Frame], max_gap_ms: float) -> dict[str, float]:
    """Gaps between changed frames, ignoring the first frame (startup declares every Task)."""
    changes = changed_frames(frames)[1:]
    if len(changes) < MIN_MANY_CHANGES:
        raise VerifyError(
            f"{len(changes)} changed frames after the first, want at least {MIN_MANY_CHANGES}"
        )
    gaps = stream.gaps_over(changes, max_gap_ms)
    if gaps:
        raise VerifyError(describe_gaps(gaps, max_gap_ms))
    max_gap = max(
        ms(cur.at_ns, prev.at_ns) for prev, cur in itertools.pairwise(changes)
    )
    return {"max_gap_ms": round(max_gap, 1), "changed_frames": len(changes)}


def scenario_for(tasks: int | None) -> Scenario:
    if tasks is None:
        return Scenario(
            QUIET_EXAMPLE, (f"--quiet={QUIET_MS}ms",), QUIET_TIMEOUT_S, verify_quiet
        )
    return Scenario(
        MANY_EXAMPLE,
        (f"--tasks={tasks}", f"--hold={MANY_HOLD_MS}ms"),
        MANY_TIMEOUT_S,
        verify_many,
    )


def attempt(
    binary: Path, scenario: Scenario, max_gap_ms: float, compare_plain: bool
) -> dict[str, float]:
    chunks, exit_code, reader_max_loop_ms = run_under_pty(
        binary, scenario.args, scenario.timeout_s
    )
    if reader_max_loop_ms >= max_gap_ms:
        raise Unprovable(f"reader stalled {reader_max_loop_ms:.0f}ms")
    if exit_code != 0:
        raise VerifyError(f"{Path(scenario.example).name} exit {exit_code}, want 0")
    frames = stream.split_frames(chunks)
    result = scenario.verify(frames, max_gap_ms)
    if compare_plain:
        verify_truth(frames, exit_code, binary, scenario)
        result = result | {"final_equals_plain": True}
    return result | {"reader_max_loop_ms": round(reader_max_loop_ms, 1)}


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument(
        "--tasks",
        type=int,
        default=None,
        help="declare N trivial Tasks in one Group instead of the quiet Writer Task",
    )
    parser.add_argument(
        "--max-gap-ms",
        type=float,
        default=DEFAULT_MAX_GAP_MS,
        help="fail when two changed frames arrive this far apart or more",
    )
    parser.add_argument(
        "--compare-plain",
        action="store_true",
        help="also require monotonic frames and a final screen equal to the Plain run",
    )
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    scenario = scenario_for(args.tasks)
    last_unprovable = ""
    with tempfile.TemporaryDirectory(prefix="verify-pty-stream-bin-") as workdir:
        try:
            binary = build_example(scenario.example, Path(workdir))
        except subprocess.CalledProcessError as err:
            print(f"FAIL quiet PTY stream: build: {err}", file=sys.stderr)
            return 1
        for number in range(1, ATTEMPTS + 1):
            try:
                result = attempt(binary, scenario, args.max_gap_ms, args.compare_plain)
            except Unprovable as err:
                last_unprovable = str(err)
                print(
                    f"RETRY quiet PTY stream: {err} (attempt {number})", file=sys.stderr
                )
                continue
            except VerifyError as err:
                print(f"FAIL quiet PTY stream: {err}", file=sys.stderr)
                return 1
            print(
                "OK quiet PTY stream: "
                + " ".join(f"{k}={v}" for k, v in result.items())
            )
            return 0
    print(
        f"FAIL quiet PTY stream: unprovable under current load "
        f"({ATTEMPTS} attempts; last: {last_unprovable})",
        file=sys.stderr,
    )
    return 1


if __name__ == "__main__":
    raise SystemExit(main())
