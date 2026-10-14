"""Unit tests for pty-stream frame splitting and owner-glyph extraction."""

from __future__ import annotations

import unittest

import ptystream_frames as stream

UP = b"\x1b[2A"
ERASE = b"\r\x1b[2K"
HIDE = b"\x1b[?25l"


def repaint(glyph: str, evidence: str = "compile unit 1 of 6") -> bytes:
    """One three-row repaint as terminal/ansi.go writes it: erase pass, then rows."""
    erase = UP + ERASE + b"\n" + ERASE + b"\n" + ERASE + UP + b"\r"
    rows = [f"{glyph}  title", f"  {glyph}  build  waiting", f"    {evidence}"]
    body = b"\n".join(ERASE + b"\x1b[36m" + r.encode() + b"\x1b[0m" for r in rows)
    return erase + body


FIRST = (
    HIDE
    + b"\r\x1b[2K\x1b[36m\xe2\xa0\x8b  title\x1b[0m\n"
    + ERASE
    + b"  \xe2\xa0\x8b  build\n"
    + ERASE
    + b"    compile unit 1 of 6"
)


class SplitFramesTest(unittest.TestCase):
    def test_first_paint_and_repaints_become_frames(self) -> None:
        chunks = [
            stream.Chunk(10, FIRST),
            stream.Chunk(60, repaint("⠙")),
            stream.Chunk(110, repaint("⠹")),
        ]
        frames = stream.split_frames(chunks)
        self.assertEqual([f.at_ns for f in frames], [10, 60, 110])
        self.assertEqual([f.owner_glyph("build") for f in frames], ["⠋", "⠙", "⠹"])
        self.assertEqual(frames[1].rows[2], "    compile unit 1 of 6")

    def test_frame_split_across_reads_arrives_with_its_last_byte(self) -> None:
        data = repaint("⠙")
        cut = len(data) - 5
        chunks = [
            stream.Chunk(10, FIRST),
            stream.Chunk(60, data[:cut]),
            stream.Chunk(75, data[cut:]),
        ]
        self.assertEqual([f.at_ns for f in stream.split_frames(chunks)], [10, 75])

    def test_repaints_batched_into_one_read_share_a_timestamp(self) -> None:
        chunks = [stream.Chunk(10, FIRST + repaint("⠙") + repaint("⠹"))]
        frames = stream.split_frames(chunks)
        self.assertEqual(len(frames), 3)
        self.assertEqual({f.at_ns for f in frames}, {10})

    def test_erase_pass_alone_is_not_a_frame(self) -> None:
        self.assertEqual(
            stream.split_frames([stream.Chunk(1, UP + ERASE + UP + b"\r")]), []
        )


class OwnerGlyphTest(unittest.TestCase):
    def test_none_without_owner_row(self) -> None:
        frame = stream.Frame(0, ("⠋  title",))
        self.assertIsNone(frame.owner_glyph("build"))

    def test_none_when_owner_row_has_settled_glyph(self) -> None:
        frame = stream.Frame(0, ("✓  build  done",))
        self.assertIsNone(frame.owner_glyph("build"))

    def test_ascii_spinner_needs_two_spaces(self) -> None:
        self.assertEqual(stream.Frame(0, ("o  build",)).owner_glyph("build"), "o")
        self.assertIsNone(stream.Frame(0, ("o build",)).owner_glyph("build"))


class GapsOverTest(unittest.TestCase):
    def test_lists_only_gaps_at_or_over_the_bound(self) -> None:
        ms = stream.NS_PER_MS
        frames = [
            stream.Frame(at_ns=t * ms, rows=("r",)) for t in (0, 50, 160, 210, 310)
        ]
        self.assertEqual(stream.gaps_over(frames, 100), [(50.0, 110.0), (210.0, 100.0)])

    def test_no_gap_over_the_bound_is_empty(self) -> None:
        ms = stream.NS_PER_MS
        frames = [stream.Frame(at_ns=t * ms, rows=("r",)) for t in (0, 50, 99)]
        self.assertEqual(stream.gaps_over(frames, 100), [])


if __name__ == "__main__":
    unittest.main()
