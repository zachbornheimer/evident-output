"""Check that a live pty stream tells the truth: rows settle forward, and the
final screen equals the Plain rows of an identical run.

Rows are read the way a person reads them: a glyph, a name (up to the first
run of two spaces), then an optional reason. Spinners and ``○`` waiting rows
are in-progress; the glyphs in SETTLED never go back.
"""

from __future__ import annotations

import re
from collections.abc import Iterable, Sequence

import ptystream_frames as stream

SETTLED = frozenset("✓✗-!■")
FINAL = SETTLED | {"○"}
SUMMARY_PREFIX = "["
COMPLETE_RE = re.compile(r"(\d+)/(\d+) complete")
DURATION_RE = re.compile(r"\s+\d+(?:\.\d+)?(?:ms|µs|us|s|m|h)$")
NAME_END_RE = re.compile(r"\s{2,}")


class TruthError(Exception):
    """The stream proves the live frames or the final screen are untruthful."""


def split_row(row: str) -> tuple[str, str] | None:
    """(glyph, name) of a glyph row, or None for any other row."""
    text = row.strip()
    if len(text) < 3 or text[1] != " ":
        return None
    return text[0], NAME_END_RE.split(text[2:].strip(), maxsplit=1)[0]


def check_monotonic(frames: Sequence[stream.Frame]) -> None:
    """No settled row reverts or changes outcome, and no completion count falls."""
    settled: dict[str, str] = {}
    completed: dict[str, int] = {}
    for number, frame in enumerate(frames):
        for row in frame.rows:
            parts = split_row(row)
            if parts is None:
                continue
            glyph, name = parts
            _check_settled(settled, name, glyph, number)
            _check_count(completed, name, row, number)


def _check_settled(settled: dict[str, str], name: str, glyph: str, number: int) -> None:
    before = settled.get(name)
    if before is None:
        if glyph in SETTLED:
            settled[name] = glyph
        return
    if glyph in SETTLED and glyph != before:
        raise TruthError(f"{name} changed from {before} to {glyph} at frame {number}")
    if glyph not in SETTLED:
        raise TruthError(f"{name} reverted from {before} to {glyph} at frame {number}")


def _check_count(completed: dict[str, int], name: str, row: str, number: int) -> None:
    match = COMPLETE_RE.search(row)
    if match is None:
        return
    done = int(match.group(1))
    before = completed.get(name, 0)
    if done < before:
        raise TruthError(
            f"{name} completion fell from {before}/{match.group(2)} to "
            f"{done}/{match.group(2)} at frame {number}"
        )
    completed[name] = done


def normalize_rows(rows: Iterable[str]) -> list[str]:
    """Sorted final rows (glyph, name, reason) and run summaries, durations dropped."""
    kept = []
    for row in rows:
        text = " ".join(row.split())
        if text[:1] in FINAL and text[1:2] == " " or text.startswith(SUMMARY_PREFIX):
            kept.append(DURATION_RE.sub("", text))
    return sorted(kept)


def check_final_matches_plain(
    final: stream.Frame, screen_exit: int, plain_rows: Iterable[str], plain_exit: int
) -> None:
    """The last pty frame holds exactly the Plain run's final rows, and the exit codes agree."""
    if screen_exit != plain_exit:
        raise TruthError(f"pty run exit {screen_exit}, Plain run exit {plain_exit}")
    screen, plain = normalize_rows(final.rows), normalize_rows(plain_rows)
    if screen == plain:
        return
    missing = [row for row in plain if row not in screen]
    extra = [row for row in screen if row not in plain]
    raise TruthError(
        f"final screen differs from Plain: missing {missing[:3]}, extra {extra[:3]}"
        f" ({len(screen)} screen rows, {len(plain)} Plain rows)"
    )
