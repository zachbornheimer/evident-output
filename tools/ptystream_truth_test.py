"""Unit tests for live-frame monotonicity and final-frame-equals-Plain checks."""

from __future__ import annotations

import sys
import unittest
from pathlib import Path

# `python3 -I` leaves the script's directory off sys.path.
if str(Path(__file__).resolve().parent) not in sys.path:
    sys.path.insert(0, str(Path(__file__).resolve().parent))

import ptystream_frames as stream
import ptystream_truth as truth


def frame(*rows: str, at_ms: int = 0) -> stream.Frame:
    return stream.Frame(at_ns=at_ms * stream.NS_PER_MS, rows=tuple(rows))


RUNNING = frame("⠋ units  1/3 complete", "   ✓ unit 1", "   ⠋ unit 2     holding")
LATER = frame("⠙ units  2/3 complete", "   ✓ unit 1", "   ✓ unit 2")
SETTLED = frame("✓ unit 1", "✓ unit 2", "✓ unit 3")


class MonotonicFramesTest(unittest.TestCase):
    def test_accepts_rows_that_only_settle_forward(self) -> None:
        truth.check_monotonic([RUNNING, LATER, SETTLED])

    def test_rejects_a_settled_row_that_reverts(self) -> None:
        reverted = frame("⠙ units  2/3 complete", "   ⠙ unit 1     holding")
        with self.assertRaisesRegex(truth.TruthError, "unit 1.*reverted"):
            truth.check_monotonic([RUNNING, reverted])

    def test_rejects_a_settled_row_that_changes_outcome(self) -> None:
        failed = frame("✗ unit 1")
        with self.assertRaisesRegex(truth.TruthError, "unit 1.*changed"):
            truth.check_monotonic([RUNNING, failed])

    def test_rejects_a_completion_count_that_falls(self) -> None:
        behind = frame("⠙ units  1/3 complete", "   ⠙ unit 3     holding")
        with self.assertRaisesRegex(truth.TruthError, r"2/3.*1/3"):
            truth.check_monotonic([RUNNING, LATER, behind])

    def test_waiting_rows_are_not_settled(self) -> None:
        waiting = frame("⠋ units  0/2 complete", "   ○ unit 2")
        started = frame("⠙ units  0/2 complete", "   ⠙ unit 2     holding")
        truth.check_monotonic([waiting, started])


class FinalMatchesPlainTest(unittest.TestCase):
    PLAIN = ("◐ units  unit 3  holding", "✓ unit 1", "✓ unit 2", "✓ unit 3")

    def test_accepts_identical_rows_in_the_same_order(self) -> None:
        truth.check_final_matches_plain(SETTLED, 0, self.PLAIN, 0)

    def test_rejects_a_reordered_pair_naming_the_index(self) -> None:
        reordered = frame("✓ unit 1", "✓ unit 3", "✓ unit 2")
        with self.assertRaisesRegex(truth.TruthError, r"row 1.*unit 3.*unit 2"):
            truth.check_final_matches_plain(reordered, 0, self.PLAIN, 0)

    def test_accepts_a_streamed_block_against_its_settled_form(self) -> None:
        streamed = (
            "◐ units  unit 2  holding",
            "◐ units  unit 1  holding",
            "✓ units",
            "   ✓ unit 1",
            "   ✓ unit 2",
        )
        settled = frame("✓ units", "   ✓ unit 1", "   ✓ unit 2")
        truth.check_final_matches_plain(settled, 0, streamed, 0)

    def test_container_row_compares_at_its_closing_position(self) -> None:
        settled = frame("   ✓ unit 1", "✓ units", "   ✓ unit 2")
        streamed = ("✓ units", "   ✓ unit 1", "   ✓ unit 2")
        with self.assertRaisesRegex(truth.TruthError, "row 0"):
            truth.check_final_matches_plain(settled, 0, streamed, 0)

    def test_streamed_block_members_compare_as_a_set(self) -> None:
        plain = (
            "◐ units  unit 2  holding",
            "   ✓ unit 3",
            "   ✓ unit 1",
            "   ✓ unit 2",
            "✓ units",
            "✓ after",
        )
        screen = frame(
            "✓ units", "   ✓ unit 1", "   ✓ unit 2", "   ✓ unit 3", "✓ after"
        )
        truth.check_final_matches_plain(screen, 0, plain, 0)

    def test_nested_streamed_blocks_settle_to_their_declared_form(self) -> None:
        plain = (
            "◐ g1",
            "   ◐ g2",
            "      ✓ g3",
            "         ✓ d1",
            "      ✓ mid",
            "   ✓ g2",
            "   ✓ top",
            "✓ g1",
            "✓ after",
        )
        screen = frame(
            "✓ g1",
            "   ✓ top",
            "   ✓ g2",
            "      ✓ mid",
            "      ✓ g3",
            "         ✓ d1",
            "✓ after",
        )
        truth.check_final_matches_plain(screen, 0, plain, 0)

    NEST_SCREEN = (
        "✓ clean caches",
        "   ✓ npm",
        "      ✓ a",
        "      ✓ b",
        "   ✓ pip",
        "      ✓ a",
        "      ✓ b",
    )

    def test_a_block_that_heads_again_after_an_interruption_is_one_block(self) -> None:
        plain = (
            "◐ clean caches",
            "   ◐ npm",
            "      ✓ a",
            "   ◐ pip",
            "      ✓ a",
            "      ✓ b",
            "   ✓ pip",
            "   ◐ npm",
            "      ✓ b",
            "   ✓ npm",
            "✓ clean caches",
        )
        truth.check_final_matches_plain(frame(*self.NEST_SCREEN), 0, plain, 0)

    def test_rejects_a_late_member_written_under_the_block_before_it(self) -> None:
        plain = (
            "◐ clean caches",
            "   ◐ npm",
            "      ✓ a",
            "   ◐ pip",
            "      ✓ a",
            "      ✓ b",
            "   ✓ pip",
            "      ✓ b",
            "   ✓ npm",
            "✓ clean caches",
        )
        with self.assertRaisesRegex(
            truth.TruthError, r"row 3.*screen '      ✓ b'.*Plain '   ✓ pip'"
        ):
            truth.check_final_matches_plain(frame(*self.NEST_SCREEN), 0, plain, 0)

    def test_rejects_a_closing_row_written_under_a_root_row(self) -> None:
        screen = frame("✓ clean caches", "   ✓ npm", "      ✓ a", "⊘ publish  declined")
        plain = (
            "◐ clean caches",
            "   ◐ npm",
            "      ✓ a",
            "⊘ publish  declined",
            "   ✓ npm",
        )
        with self.assertRaises(truth.TruthError):
            truth.check_final_matches_plain(screen, 0, plain, 0)

    def test_accepts_a_closing_row_headed_again_after_a_root_row(self) -> None:
        screen = frame("✓ clean caches", "   ✓ npm", "      ✓ a", "⊘ publish  declined")
        plain = (
            "◐ clean caches",
            "   ◐ npm",
            "      ✓ a",
            "⊘ publish  declined",
            "◐ clean caches",
            "   ✓ npm",
            "✓ clean caches",
        )
        truth.check_final_matches_plain(screen, 0, plain, 0)

    def test_an_unstreamed_container_keeps_its_member_order(self) -> None:
        plain = ("✓ seq", "   ✓ one", "   ✓ two")
        screen = frame("✓ seq", "   ✓ two", "   ✓ one")
        with self.assertRaisesRegex(truth.TruthError, "row 1"):
            truth.check_final_matches_plain(screen, 0, plain, 0)

    def test_rejects_a_dependent_printed_above_its_block(self) -> None:
        plain = ("◐ eg", "   ✓ fast", "✓ eg", "- depEx  not started")
        screen = frame("- depEx  not started", "✓ eg", "   ✓ fast")
        with self.assertRaisesRegex(truth.TruthError, "row 0"):
            truth.check_final_matches_plain(screen, 0, plain, 0)

    def test_rejects_held_rows_in_declaration_not_release_order(self) -> None:
        plain = ("✓ g", "✓ chain", "✓ depdep")
        screen = frame("✓ g", "✓ depdep", "✓ chain")
        with self.assertRaisesRegex(truth.TruthError, r"row 1.*depdep.*chain"):
            truth.check_final_matches_plain(screen, 0, plain, 0)

    def test_rejects_a_free_root_row_above_a_collection_plain_wrote_first(self) -> None:
        plain = ("✓ fast group", "   ✓ f1", "   ✓ f2", "✓ slow task")
        screen = frame("✓ slow task", "✓ fast group", "   ✓ f1", "   ✓ f2")
        with self.assertRaisesRegex(truth.TruthError, r"row 0.*slow task.*fast group"):
            truth.check_final_matches_plain(screen, 0, plain, 0)

    def test_accepts_a_free_root_row_after_a_streamed_block_in_durable_order(
        self,
    ) -> None:
        plain = ("◐ units", "   ✓ unit 2", "   ✓ unit 1", "✓ units", "✓ late task")
        screen = frame("✓ units", "   ✓ unit 1", "   ✓ unit 2", "✓ late task")
        truth.check_final_matches_plain(screen, 0, plain, 0)

    def test_a_heartbeat_is_not_a_block(self) -> None:
        plain = ("◐ build  compiling", "✓ build")
        truth.check_final_matches_plain(frame("✓ build"), 0, plain, 0)

    def test_ignores_timing_suffixes(self) -> None:
        timed = frame("✓ unit 1   12ms", "✓ unit 2", "✓ unit 3")
        truth.check_final_matches_plain(timed, 0, self.PLAIN, 0)

    def test_rejects_a_row_missing_from_the_final_screen(self) -> None:
        short = frame("✓ unit 1", "✓ unit 2")
        with self.assertRaisesRegex(truth.TruthError, "unit 3"):
            truth.check_final_matches_plain(short, 0, self.PLAIN, 0)

    def test_rejects_a_different_outcome(self) -> None:
        failed = frame("✓ unit 1", "✓ unit 2", "✗ unit 3")
        with self.assertRaisesRegex(truth.TruthError, "unit 3"):
            truth.check_final_matches_plain(failed, 0, self.PLAIN, 0)

    def test_rejects_a_different_exit_code(self) -> None:
        with self.assertRaisesRegex(truth.TruthError, "exit"):
            truth.check_final_matches_plain(SETTLED, 0, self.PLAIN, 2)

    def test_run_summary_rows_compare_by_verdict(self) -> None:
        screen = frame("✓ a", "[succeeded · partial]  probe-dag")
        plain = ["✓ a", "[succeeded · partial]  probe-dag"]
        truth.check_final_matches_plain(screen, 0, plain, 0)


class LiveTerminal:
    """The bytes internal/terminal/ansi.go writes, as a pty delivers them.

    A live region repaints under the durable rows that settled above it: erase
    the region (cursor up, erase each row, cursor up), write the settled rows
    with CR LF, then paint the rows still running.
    """

    def __init__(self) -> None:
        self.chunks: list[stream.Chunk] = []
        self.live = 0

    def _write(self, text: str) -> None:
        self.chunks.append(stream.Chunk(len(self.chunks), text.encode()))

    def _erase_live(self) -> None:
        if self.live > 1:
            self._write(f"\x1b[{self.live - 1}A")
        for row in range(self.live):
            self._write("\r\x1b[2K" + ("\r\n" if row < self.live - 1 else ""))
        if self.live > 1:
            self._write(f"\x1b[{self.live - 1}A")
        self._write("\r")
        self.live = 0

    def paint(self, *rows: str) -> None:
        self._erase_live()
        self._write("\r\n".join("\r\x1b[2K" + row for row in rows))
        self.live = len(rows)

    def settle(self, *rows: str) -> None:
        self._erase_live()
        self._write("".join(row + "\r\n" for row in rows))

    def final_screen(self) -> stream.Frame:
        return stream.final_screen(self.chunks)


class ScrollbackFixturesTest(unittest.TestCase):
    """Programs whose rows settle one by one, or fold, leave more than the last repaint."""

    NODEFINE_PLAIN = (
        "\u25d0 progress  3/3",
        "- problem2       not started",
        "   \u2514\u2500 x",
        "- after-problem  not started \u2014 problem2 not started",
        "- progress       not started",
        "- deploy         not started \u2014 progress not started",
        "- bare           not started",
        "- fact           not started",
        "- problem        not started",
        "   \u2514\u2500 heads up",
        "- verify         not started",
        "- summary        not started",
        "",
        "[succeeded \u00b7 partial]  nodefine",
    )

    def nodefine_terminal(self) -> LiveTerminal:
        """Tasks with no Define settle NotStarted one at a time, not in declaration order."""
        waiting = [
            "bare",
            "fact",
            "problem",
            "verify",
            "summary",
            "after-problem",
            "problem2",
        ]
        waiting += ["deploy", "progress"]
        term = LiveTerminal()
        term.paint(*(f"\u25cb {name}" for name in waiting))
        for name, rows in (
            ("problem2", ["- problem2       not started", "   \u2514\u2500 x"]),
            (
                "after-problem",
                ["- after-problem  not started \u2014 problem2 not started"],
            ),
            ("progress", ["- progress       not started"]),
            ("deploy", ["- deploy         not started \u2014 progress not started"]),
            ("bare", ["- bare           not started"]),
            ("fact", ["- fact           not started"]),
            ("problem", ["- problem        not started", "   \u2514\u2500 heads up"]),
            ("verify", ["- verify         not started"]),
            ("summary", ["- summary        not started"]),
        ):
            waiting.remove(name)
            term.settle(*rows)
            if waiting:
                term.paint(*(f"\u25cb {n}" for n in waiting))
        term.settle("", "[succeeded \u00b7 partial]  nodefine")
        return term

    def test_the_final_screen_holds_every_row_that_settled_into_scrollback(
        self,
    ) -> None:
        term = self.nodefine_terminal()
        last_repaint = stream.split_frames(term.chunks)[-1]
        self.assertLess(len(last_repaint.rows), len(self.NODEFINE_PLAIN) - 1)
        truth.check_final_matches_plain(term.final_screen(), 0, self.NODEFINE_PLAIN, 0)

    def test_a_progress_line_before_the_row_settles_is_not_a_block(self) -> None:
        plain = ("\u25d0 slow  2/3", "\u2713 quick", "\u2713 slow")
        truth.check_final_matches_plain(
            frame("\u2713 quick", "\u2713 slow"), 0, plain, 0
        )

    SLOWFOLD_SCREEN = (
        "\u2713 branches",
        "   \u25cb excluded 2 (protected)",
        "   \u2713 feature-01",
        "   \u2713 feature-02",
        "   \u2713 feature-04",
    )

    def slowfold_terminal(self) -> LiveTerminal:
        """A Group of five branches, two Excluded, written whole at the end of the run."""
        term = LiveTerminal()
        term.paint("\u280b branches", "   \u280b feature-00", "   \u280b feature-01")
        term.paint("\u2819 branches", "   \u2819 feature-02", "   \u2819 feature-04")
        term.settle(*self.SLOWFOLD_SCREEN)
        return term

    def test_a_folded_tally_in_a_streamed_block_agrees_with_the_final_screen(
        self,
    ) -> None:
        plain = (
            "\u25d0 branches",
            "   \u2713 feature-01",
            "   \u2713 feature-04",
            "   \u2713 feature-02",
            "\u2713 branches",
            "   \u25cb excluded 2 (protected)",
        )
        truth.check_final_matches_plain(
            self.slowfold_terminal().final_screen(), 0, plain, 0
        )

    def test_rejects_plain_that_streams_each_excluded_member_as_a_row(self) -> None:
        unfolded = (
            "\u25d0 branches",
            "   \u25cb feature-03  protected",
            "   \u2713 feature-01",
            "   \u25cb feature-00  protected",
            "   \u2713 feature-04",
            "   \u2713 feature-02",
            "\u2713 branches",
        )
        with self.assertRaisesRegex(truth.TruthError, "excluded 2"):
            truth.check_final_matches_plain(
                self.slowfold_terminal().final_screen(), 0, unfolded, 0
            )


if __name__ == "__main__":
    unittest.main()
