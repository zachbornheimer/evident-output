#!/usr/bin/env python3
"""Prove a quiet Writer-backed Task keeps its live row changing in a real PTY.

Builds examples/quiet-writer, captures it with scripts/test-evo.py in tmux,
then checks the timed frames: the owner row stays Running and quiet, its
Writer evidence is on screen, and the live frame keeps changing.

A tmux capture costs ~20-40ms, so a changed frame is seen up to one capture
late. The stale interval each change ends is therefore measured as the span
the capture proved the frame unchanged: first sight of a frame to the last
capture that still showed it. Any such span of MAX_GAP_MS or more fails. A
capture whose own sampling gap reaches MAX_GAP_MS cannot prove the bound, so
it is retried (up to CAPTURE_ATTEMPTS) rather than passed.

test-evo.py exits HARNESS_ASSERTION_EXIT when its own mandatory-frame
assertions fail, which host contention can cause on its own. Those samples
are auxiliary: when the capture directory holds meta.json, this verifier still
applies its strict timed-frame proof and reports the harness exit beside it.

Prints the retained frame directory on success and on failure.
"""

from __future__ import annotations

import argparse
import json
import subprocess
import sys
import tempfile
from dataclasses import dataclass
from datetime import datetime
from pathlib import Path

import testevo_frames

SCRIPTS = Path(__file__).resolve().parent
ROOT = SCRIPTS.parent
EXAMPLE = "./examples/quiet-writer"
OWNER_ROW = "build"
QUIET_PHASE = "waiting"
EVIDENCE_LINES = 6
EVIDENCE = tuple(f"compile unit {n} of {EVIDENCE_LINES}" for n in range(1, EVIDENCE_LINES + 1))
QUIET_MS = 900
MIN_QUIET_SPAN_MS = 700
MAX_GAP_MS = 100
MIN_CHANGED_FRAMES = 8
MIN_QUIET_CHANGES = 6
CAPTURE_INTERVAL_MS = 20
CAPTURE_TIMEOUT_S = 10
CAPTURE_ATTEMPTS = 3
CHILD_EXIT_NAME = "child_exit"
META_NAME = "meta.json"
HARNESS_ASSERTION_EXIT = 1
STAMP_FORMAT = "%Y-%m-%d %H:%M:%S.%f"


@dataclass(frozen=True)
class Frame:
    name: str
    at: datetime
    text: str

    @property
    def running(self) -> bool:
        spinner = testevo_frames.leading_spinner(self.text)
        return spinner is not None and OWNER_ROW in self.text

    @property
    def quiet(self) -> bool:
        return self.running and QUIET_PHASE in self.text


class VerifyError(Exception):
    """The capture proves the requirement is not met."""


class InconclusiveCapture(Exception):
    """The capture sampled too sparsely to prove or disprove the bound."""


def ms(later: datetime, earlier: datetime) -> float:
    return (later - earlier).total_seconds() * 1000


def parse_stamp(stamp: str) -> datetime:
    # test-evo.py stamps "YYYY-mm-dd HH:MM:SS.hh ZONE"; the zone is constant per run.
    return datetime.strptime(stamp.rsplit(" ", 1)[0], STAMP_FORMAT)


def build_example(workdir: Path) -> Path:
    binary = workdir / "quiet-writer"
    subprocess.run(
        ["go", "build", "-buildvcs=false", "-o", str(binary), EXAMPLE],
        cwd=ROOT,
        check=True,
    )
    return binary


def capture(binary: Path, frames_dir: Path) -> int:
    """Run the tmux harness and return its exit status.

    Only a zero exit, or the auxiliary assertion exit with a written capture,
    passes through; the timed frames decide the requirement either way.
    """
    command = [
        sys.executable,
        str(SCRIPTS / "test-evo.py"),
        "--start-dir",
        str(ROOT),
        "--timeout-s",
        str(CAPTURE_TIMEOUT_S),
        "--screenshot-interval-ms",
        str(CAPTURE_INTERVAL_MS),
        "--output-frames",
        str(frames_dir),
        "--",
        str(binary),
        f"--quiet={QUIET_MS}ms",
    ]
    code = subprocess.run(command, check=False).returncode
    auxiliary = code == HARNESS_ASSERTION_EXIT and (frames_dir / META_NAME).is_file()
    if code != 0 and not auxiliary:
        raise subprocess.CalledProcessError(code, command)
    return code


def child_exit(frames_dir: Path) -> str:
    # Read the file, not meta.json: test-evo.py can snapshot it mid-write.
    path = frames_dir / CHILD_EXIT_NAME
    return path.read_text(encoding="utf-8").strip() if path.is_file() else "missing"


def load_timed_frames(frames_dir: Path) -> list[Frame]:
    code = child_exit(frames_dir)
    if code != "0":
        raise VerifyError(f"quiet-writer exit {code}, want 0")
    meta = json.loads((frames_dir / META_NAME).read_text(encoding="utf-8"))
    timed = sorted(
        (f for f in meta["frames"] if f["kind"] == testevo_frames.KIND_TIMED),
        key=lambda f: f["index"],
    )
    return [
        Frame(
            name=f["name"],
            at=parse_stamp(f["captured_at"]),
            text=testevo_frames.visible_text(
                (frames_dir / f"{f['name']}.txt").read_text(encoding="utf-8")
            ),
        )
        for f in timed
    ]


def running_window(frames: list[Frame]) -> list[Frame]:
    """Frames from the first Running owner row through the last one.

    The child can settle right after the final capture, so a capture that ends
    while the row is still Running is valid evidence of the active interval.
    If a non-running frame is observed, it closes the window as its boundary
    only: it proves the Running interval ended, not how the row settled.
    """
    start = next((i for i, f in enumerate(frames) if f.running), None)
    if start is None:
        raise VerifyError("no frame shows the Running owner row")
    end = next(
        (i for i in range(start + 1, len(frames)) if not frames[i].running), len(frames)
    )
    window = frames[start : end + 1]
    if len(window) < 2:
        raise VerifyError("only one frame shows the Running owner row")
    return window


def unchanged_runs(window: list[Frame]) -> list[list[Frame]]:
    """Consecutive captures showing identical text, one run per changed frame."""
    runs = [[window[0]]]
    for frame in window[1:]:
        if frame.text == runs[-1][-1].text:
            runs[-1].append(frame)
        else:
            runs.append([frame])
    return runs


def span_ms(frames: list[Frame]) -> float:
    return ms(frames[-1].at, frames[0].at)


def verify(frames: list[Frame]) -> dict[str, int]:
    window = running_window(frames)
    capture_gap = max(ms(cur.at, prev.at) for prev, cur in zip(window, window[1:]))
    if capture_gap >= MAX_GAP_MS:
        raise InconclusiveCapture(f"capture sampling gap {capture_gap:.0f}ms")
    runs = unchanged_runs(window)
    if len(runs) < MIN_CHANGED_FRAMES:
        raise VerifyError(f"{len(runs)} changed frames, want at least {MIN_CHANGED_FRAMES}")
    stalest = max(runs, key=span_ms)
    if span_ms(stalest) >= MAX_GAP_MS:
        raise VerifyError(
            f"frame unchanged for {span_ms(stalest):.0f}ms "
            f"({stalest[0].name} -> {stalest[-1].name}), want <{MAX_GAP_MS}ms"
        )
    # A boundary frame is never quiet, so every quiet frame is Running in the window.
    quiet = [run[0] for run in runs if run[0].quiet]
    if len(quiet) < MIN_QUIET_CHANGES:
        error = (
            f"{len(quiet)} changed frames while quiet, want at least {MIN_QUIET_CHANGES}"
        )
        if window[-1].running:
            raise InconclusiveCapture(error + "; capture ended while still Running")
        raise VerifyError(error)
    quiet_span = span_ms(quiet)
    if quiet_span < MIN_QUIET_SPAN_MS:
        error = (
            f"quiet changing frames span {quiet_span:.0f}ms, want at least {MIN_QUIET_SPAN_MS}ms"
        )
        if window[-1].running:
            raise InconclusiveCapture(error + "; capture ended while still Running")
        raise VerifyError(error)
    for frame in quiet:
        missing = [line for line in EVIDENCE if line not in frame.text]
        if missing:
            raise VerifyError(f"quiet frame {frame.name} lost Writer evidence {missing[0]!r}")
    return {
        "changed_frames": len(runs),
        "quiet_changed_frames": len(quiet),
        "quiet_span_ms": round(quiet_span),
        "max_unchanged_ms": round(span_ms(stalest)),
        "max_capture_gap_ms": round(capture_gap),
    }


def attempt(binary: Path, frames_dir: Path) -> dict[str, int]:
    harness_exit = capture(binary, frames_dir)
    return verify(load_timed_frames(frames_dir)) | {"harness_exit": harness_exit}


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument(
        "--output-frames", type=Path, help="frame root directory (default: new temp dir)"
    )
    args = parser.parse_args()
    root = (
        args.output_frames or Path(tempfile.mkdtemp(prefix="verify-quiet-pty-"))
    ).resolve()
    last_inconclusive = ""
    with tempfile.TemporaryDirectory(prefix="quiet-writer-bin-") as workdir:
        try:
            binary = build_example(Path(workdir))
        except subprocess.CalledProcessError as err:
            print(f"FAIL quiet PTY cadence: build: {err}", file=sys.stderr)
            return 1
        for number in range(1, CAPTURE_ATTEMPTS + 1):
            frames_dir = root / f"attempt-{number}"
            try:
                result = attempt(binary, frames_dir)
            except InconclusiveCapture as err:
                last_inconclusive = str(err)
                print(f"RETRY quiet PTY cadence: {err} (frames: {frames_dir})", file=sys.stderr)
                continue
            except (VerifyError, subprocess.CalledProcessError) as err:
                print(f"FAIL quiet PTY cadence: {err} (frames: {frames_dir})", file=sys.stderr)
                return 1
            summary = " ".join(f"{key}={value}" for key, value in result.items())
            print(f"OK quiet PTY cadence: {summary} frames={frames_dir}")
            return 0
    print(
        f"FAIL quiet PTY cadence: {CAPTURE_ATTEMPTS} captures too sparse to prove "
        f"<{MAX_GAP_MS}ms (frames: {root}; last: {last_inconclusive})",
        file=sys.stderr,
    )
    return 1


if __name__ == "__main__":
    raise SystemExit(main())
