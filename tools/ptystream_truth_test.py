"""Unit tests for live-frame monotonicity and final-frame-equals-Plain checks."""

from __future__ import annotations

import unittest

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


if __name__ == "__main__":
    unittest.main()
