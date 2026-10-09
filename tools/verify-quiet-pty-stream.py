#!/usr/bin/env python3
"""Prove a quiet Writer-backed Task keeps its live frame changing, from the pty stream.

Builds examples/quiet-writer and runs it under a real pty (no tmux). The reader
timestamps every read of the pty master with a monotonic clock, rebuilds the
live frames from the byte stream (ptystream_frames), and checks the quiet
interval, from the last Writer evidence line to the settle:

- every gap between frames whose owner-row spinner glyph differs is under MAX_GAP_MS;
- no frame stays unchanged for MAX_GAP_MS or more;
- at least MIN_QUIET_CHANGES changed frames arrive;
- the owner row stays named, all evidence lines stay on screen, no fake progress.

A reader that was itself descheduled for MAX_GAP_MS or more cannot prove the
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
from pathlib import Path

import ptystream_frames as stream

ROOT = Path(__file__).resolve().parent.parent
EXAMPLE = "./examples/quiet-writer"
OWNER_ROW = "build"
EVIDENCE_LINES = 6
EVIDENCE = tuple(
    f"compile unit {n} of {EVIDENCE_LINES}" for n in range(1, EVIDENCE_LINES + 1)
)
QUIET_MS = 900
MAX_GAP_MS = 100
MIN_QUIET_CHANGES = 6
ATTEMPTS = 3
RUN_TIMEOUT_S = 10
READ_POLL_S = 0.002
READ_SIZE = 65536
PTY_ROWS, PTY_COLS = 24, 100
TERM_VALUE = "xterm-256color"
FAKE_PROGRESS_RE = re.compile(r"\d+\s*%|[█░▓▒■□]")


class VerifyError(Exception):
    """The stream proves the requirement is not met."""


class Unprovable(Exception):
    """The reader was too slow to prove or disprove the bound."""


def build_example(workdir: Path) -> Path:
    binary = workdir / "quiet-writer"
    subprocess.run(
        ["go", "build", "-buildvcs=false", "-o", str(binary), EXAMPLE],
        cwd=ROOT,
        check=True,
    )
    return binary


def run_under_pty(binary: Path) -> tuple[list[stream.Chunk], int, float]:
    """Return the timestamped reads, the child's exit status, and the reader's worst loop gap in ms."""
    pid, master = pty.fork()
    if pid == 0:
        fcntl.ioctl(
            0, termios.TIOCSWINSZ, struct.pack("HHHH", PTY_ROWS, PTY_COLS, 0, 0)
        )
        os.environ["TERM"] = TERM_VALUE
        os.execv(str(binary), [str(binary), f"--quiet={QUIET_MS}ms"])
    chunks: list[stream.Chunk] = []
    deadline = time.monotonic() + RUN_TIMEOUT_S
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


def verify(frames: list[stream.Frame]) -> dict[str, float]:
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
    max_gap = max(
        ms(cur.at_ns, prev.at_ns) for prev, cur in itertools.pairwise(changes)
    )
    if max_gap >= MAX_GAP_MS:
        raise VerifyError(f"frames changed {max_gap:.0f}ms apart, want <{MAX_GAP_MS}ms")
    stale = stalest_run_ms(window)
    if stale >= MAX_GAP_MS:
        raise VerifyError(
            f"a frame stayed unchanged {stale:.0f}ms, want <{MAX_GAP_MS}ms"
        )
    return {"max_gap_ms": round(max_gap, 1), "changed_frames": len(quiet_changes)}


def attempt(binary: Path) -> dict[str, float]:
    chunks, exit_code, reader_max_loop_ms = run_under_pty(binary)
    if reader_max_loop_ms >= MAX_GAP_MS:
        raise Unprovable(f"reader stalled {reader_max_loop_ms:.0f}ms")
    if exit_code != 0:
        raise VerifyError(f"quiet-writer exit {exit_code}, want 0")
    result = verify(stream.split_frames(chunks))
    return result | {"reader_max_loop_ms": round(reader_max_loop_ms, 1)}


def main() -> int:
    argparse.ArgumentParser(description=__doc__.splitlines()[0]).parse_args()
    last_unprovable = ""
    with tempfile.TemporaryDirectory(prefix="quiet-writer-bin-") as workdir:
        try:
            binary = build_example(Path(workdir))
        except subprocess.CalledProcessError as err:
            print(f"FAIL quiet PTY stream: build: {err}", file=sys.stderr)
            return 1
        for number in range(1, ATTEMPTS + 1):
            try:
                result = attempt(binary)
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
