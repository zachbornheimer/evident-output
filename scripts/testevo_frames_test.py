"""Unit tests for test-evo frame naming and schedule helpers."""

from __future__ import annotations

import unittest

import testevo_frames as frames


class PadWidthTest(unittest.TestCase):
    def test_single_digit(self) -> None:
        self.assertEqual(frames.pad_width(1), 1)
        self.assertEqual(frames.pad_width(9), 1)

    def test_two_digits(self) -> None:
        self.assertEqual(frames.pad_width(10), 2)
        self.assertEqual(frames.pad_width(15), 2)

    def test_zero_is_one(self) -> None:
        self.assertEqual(frames.pad_width(0), 1)


class NumberedStemTest(unittest.TestCase):
    def test_unpadded(self) -> None:
        self.assertEqual(frames.numbered_stem("timed", 1, 1), "timed-1")
        self.assertEqual(frames.numbered_stem("mandatory", 5, 1), "mandatory-5")

    def test_padded(self) -> None:
        self.assertEqual(frames.numbered_stem("timed", 1, 2), "timed-01")
        self.assertEqual(frames.numbered_stem("timed", 12, 2), "timed-12")


class FooterTest(unittest.TestCase):
    def test_two_lines(self) -> None:
        line1, line2 = frames.footer_lines(
            "2026-09-08 11:35:33.33 EDT",
            "mandatory-4",
            "200ms after command · spinner should differ from mandatory-3?",
        )
        self.assertEqual(
            line1,
            "[screenshot taken by test-evo.py at 2026-09-08 11:35:33.33 EDT]",
        )
        self.assertIn("mandatory-4", line2)
        self.assertIn("spinner should differ from mandatory-3?", line2)


class WakeTest(unittest.TestCase):
    def test_sleeps_until_next_mandatory(self) -> None:
        got = frames.next_wake(
            1.0,
            mandatory_due=[1.1, 1.2],
            next_timed=1.3,
            deadline=10.0,
        )
        self.assertAlmostEqual(got, 0.1)

    def test_zero_when_due_now(self) -> None:
        got = frames.next_wake(
            1.0,
            mandatory_due=[0.9],
            next_timed=1.3,
            deadline=10.0,
        )
        self.assertEqual(got, 0.0)


class VisibleTextTest(unittest.TestCase):
    def test_strips_blank_edges_and_ansi(self) -> None:
        raw = "\n\n\x1b[36m⠼  zq\x1b[0m\n\n"
        self.assertEqual(frames.visible_text(raw), "⠼  zq")
        self.assertEqual(frames.leading_spinner(raw), "⠼")

    def test_empty_has_no_spinner(self) -> None:
        self.assertEqual(frames.visible_text("\n\n"), "")
        self.assertIsNone(frames.leading_spinner("\n\n"))


def _mandatory(index: int, text: str) -> frames.FrameRecord:
    return frames.FrameRecord(
        kind=frames.KIND_MANDATORY,
        index=index,
        captured_at="t",
        reason="x",
        text=text,
    )


class AssertionTest(unittest.TestCase):
    def test_all_blank_skips(self) -> None:
        checks = frames.evaluate_mandatory_assertions(
            [_mandatory(i, "\n") for i in (1, 2, 3, 4, 5)]
        )
        by_name = {c.name: c for c in checks}
        self.assertEqual(by_name["mandatory-2..5 nonempty not identical"].status, "SKIP")
        self.assertEqual(by_name["mandatory-4 vs 5 spinner moved"].status, "SKIP")

    def test_frozen_spinner_fails_vary_and_4_vs_5(self) -> None:
        checks = frames.evaluate_mandatory_assertions(
            [
                _mandatory(2, ""),
                _mandatory(3, ""),
                _mandatory(4, "⠼  zq\n"),
                _mandatory(5, "⠼  zq\n"),
            ]
        )
        by_name = {c.name: c for c in checks}
        self.assertEqual(by_name["mandatory-2..5 nonempty not identical"].status, "FAIL")
        self.assertEqual(by_name["mandatory-2 vs 3 spinner moved"].status, "SKIP")
        self.assertEqual(by_name["mandatory-3 vs 4 spinner moved"].status, "SKIP")
        self.assertEqual(by_name["mandatory-4 vs 5 spinner moved"].status, "FAIL")
        self.assertTrue(frames.assertions_failed(checks))

    def test_spinner_advanced_ok(self) -> None:
        checks = frames.evaluate_mandatory_assertions(
            [
                _mandatory(2, ""),
                _mandatory(3, "⠋  zq\n"),
                _mandatory(4, "⠙  zq\n"),
                _mandatory(5, "⠹  zq\n"),
            ]
        )
        by_name = {c.name: c for c in checks}
        self.assertEqual(by_name["mandatory-2..5 nonempty not identical"].status, "OK")
        self.assertEqual(by_name["mandatory-3 vs 4 spinner moved"].status, "OK")
        self.assertEqual(by_name["mandatory-4 vs 5 spinner moved"].status, "OK")
        self.assertFalse(frames.assertions_failed(checks))

    def test_same_spinner_different_body_fails_spinner_only(self) -> None:
        checks = frames.evaluate_mandatory_assertions(
            [
                _mandatory(4, "⠼  zq\n"),
                _mandatory(5, "⠼  zq\nphase two\n"),
            ]
        )
        by_name = {c.name: c for c in checks}
        self.assertEqual(by_name["mandatory-2..5 nonempty not identical"].status, "OK")
        self.assertEqual(by_name["mandatory-4 vs 5 spinner moved"].status, "FAIL")


if __name__ == "__main__":
    unittest.main()
