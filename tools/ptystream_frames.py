"""Split a live-region pty byte stream into timestamped frames.

terminal/ansi.go repaints the live region as: cursor-up (CSI n A) to the
region's first row, erase each row, cursor-up again, then the new rows. Every
repaint of a multi-row region therefore carries CSI n A twice, and the text
between two such sequences is either the (blank) erase pass or one frame.
Splitting on that sequence and dropping the blank pieces yields the frames.

A frame is only the live region. The screen a person is left with also holds
the rows that settled into scrollback one by one, so ``final_screen`` replays
the whole stream through a terminal emulation (``Screen``) and returns what
the terminal shows with its scrollback.
"""

from __future__ import annotations

import bisect
import itertools
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


class Screen:
    """The terminal a pty stream paints, scrollback included.

    The renderer moves with CR, LF, cursor-up (CSI n A) and erase-line
    (CSI n K) and with nothing else that changes the text, so those are the
    sequences this acts on; colours, cursor visibility and every other CSI or
    OSC sequence are dropped. Rows never scroll off: the result is the screen
    after scrolling back to the first line the run wrote.
    """

    CSI_RE = re.compile(r"\x1b\[([0-9;?]*)([ -/]*)([@-~])")
    OSC_RE = re.compile(r"\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)")

    def __init__(self) -> None:
        self.lines: list[list[str]] = [[]]
        self.row = 0
        self.col = 0

    def feed(self, text: str) -> None:
        i = 0
        while i < len(text):
            char = text[i]
            if char == "\x1b":
                i = self._escape(text, i)
                continue
            if char == "\r":
                self.col = 0
            elif char == "\n":
                self.row += 1
                self._ensure_row()
            elif char >= " ":
                self._put(char)
            i += 1

    def rows(self) -> tuple[str, ...]:
        """The non-blank rows, scrollback first, trailing spaces dropped."""
        rows = ("".join(line).rstrip() for line in self.lines)
        return tuple(row for row in rows if row)

    def _escape(self, text: str, at: int) -> int:
        csi = self.CSI_RE.match(text, at)
        if csi is not None:
            self._csi(csi.group(1), csi.group(3))
            return csi.end()
        osc = self.OSC_RE.match(text, at)
        return osc.end() if osc is not None else at + 2

    def _csi(self, params: str, final: str) -> None:
        count = int(params) if params.isdigit() else None
        if final == "A":
            self.row = max(0, self.row - (count or 1))
        elif final == "K":
            self._erase_line(count or 0)

    def _erase_line(self, mode: int) -> None:
        self._ensure_row()
        line = self.lines[self.row]
        if mode == 0:
            del line[self.col :]
        elif mode == 1:
            line[: self.col + 1] = " " * min(self.col + 1, len(line))
        else:
            line.clear()

    def _put(self, char: str) -> None:
        self._ensure_row()
        line = self.lines[self.row]
        line.extend(" " * (self.col - len(line)))
        if self.col < len(line):
            line[self.col] = char
        else:
            line.append(char)
        self.col += 1

    def _ensure_row(self) -> None:
        while len(self.lines) <= self.row:
            self.lines.append([])


def final_screen(chunks: list[Chunk]) -> Frame:
    """The screen after the whole stream: scrollback and live region together.

    The last repaint holds only the live region, so it misses every row that
    settled into scrollback before the end of the run.
    """
    screen = Screen()
    screen.feed(
        b"".join(chunk.data for chunk in chunks).decode("utf-8", errors="replace")
    )
    return Frame(at_ns=chunks[-1].at_ns if chunks else 0, rows=screen.rows())


def gaps_over(changes: list[Frame], max_gap_ms: float) -> list[tuple[float, float]]:
    """(start offset ms, gap ms) of each gap between consecutive frames at or over the bound."""
    origin = changes[0].at_ns
    gaps = (
        ((prev.at_ns - origin) / NS_PER_MS, (cur.at_ns - prev.at_ns) / NS_PER_MS)
        for prev, cur in itertools.pairwise(changes)
    )
    return [(start, gap) for start, gap in gaps if gap >= max_gap_ms]
