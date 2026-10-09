"""Split a live-region pty byte stream into timestamped frames.

terminal/ansi.go repaints the live region as: cursor-up (CSI n A) to the
region's first row, erase each row, cursor-up again, then the new rows. Every
repaint of a multi-row region therefore carries CSI n A twice, and the text
between two such sequences is either the (blank) erase pass or one frame.
Splitting on that sequence and dropping the blank pieces yields the frames.
"""

from __future__ import annotations

import bisect
import re
from dataclasses import dataclass

import testevo_frames

CURSOR_UP_RE = re.compile(rb"\x1b\[\d+A")
NS_PER_MS = 1_000_000


@dataclass(frozen=True)
class Chunk:
    """One os.read result and the monotonic time it returned."""

    at_ns: int
    data: bytes


@dataclass(frozen=True)
class Frame:
    """The rows of one repaint and when its last byte arrived."""

    at_ns: int
    rows: tuple[str, ...]

    def owner_glyph(self, owner: str) -> str | None:
        """Spinner glyph on the first row naming owner, or None if none shows."""
        row = next((r.lstrip() for r in self.rows if owner in r), None)
        return None if row is None else testevo_frames.leading_spinner(row)


def frame_rows(piece: bytes) -> tuple[str, ...]:
    text = testevo_frames.strip_ansi(piece.decode("utf-8", errors="replace"))
    rows = (row.replace("\r", "").rstrip() for row in text.split("\n"))
    return tuple(row for row in rows if row)


def split_frames(chunks: list[Chunk]) -> list[Frame]:
    stream = b"".join(chunk.data for chunk in chunks)
    ends: list[int] = []
    total = 0
    for chunk in chunks:
        total += len(chunk.data)
        ends.append(total)

    def arrival_ns(last_byte: int) -> int:
        return chunks[bisect.bisect_right(ends, last_byte)].at_ns

    bounds = [0]
    for match in CURSOR_UP_RE.finditer(stream):
        bounds += [match.start(), match.end()]
    bounds.append(len(stream))
    frames = []
    for start, end in zip(bounds[::2], bounds[1::2]):
        rows = frame_rows(stream[start:end])
        if rows:
            frames.append(Frame(at_ns=arrival_ns(end - 1), rows=rows))
    return frames
