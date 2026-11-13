"""Frame naming, footer copy, capture schedule, and post-run assertions."""

from __future__ import annotations

import re
from dataclasses import dataclass

KIND_TIMED = "timed"
KIND_MANDATORY = "mandatory"

MANDATORY_BEFORE_COMMAND = 1
MANDATORY_GAP_S = 0.100
MANDATORY_AFTER_SUBMIT: tuple[tuple[int, str], ...] = (
    (2, "immediately after command"),
    (3, "100ms after command"),
    (4, "200ms after command · spinner should differ from mandatory-3?"),
    (5, "300ms after command · spinner should differ from mandatory-4?"),
)
MANDATORY_BEFORE_REASON = "before command"
MANDATORY_COMPARE_INDEXES = (2, 3, 4, 5)
SPINNER_PAIRS = ((2, 3), (3, 4), (4, 5))

STATUS_OK = "OK"
STATUS_FAIL = "FAIL"
STATUS_SKIP = "SKIP"

UNICODE_SPINNERS = frozenset("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")
ASCII_SPINNERS = frozenset(".oO@")
ANSI_RE = re.compile(r"\x1b\[[0-9;?]*[ -/]*[@-~]")


@dataclass(frozen=True)
class FrameRecord:
    kind: str
    index: int
    captured_at: str
    reason: str
    text: str

    def stem(self, width: int) -> str:
        return numbered_stem(self.kind, self.index, width)


def pad_width(count: int) -> int:
    if count <= 0:
        return 1
    return len(str(count))


def numbered_stem(kind: str, index: int, width: int) -> str:
    return f"{kind}-{index:0{width}d}"


def timed_reason(interval_s: float) -> str:
    return f"every {interval_s:g}s"


def footer_lines(captured_at: str, name: str, reason: str) -> tuple[str, str]:
    line1 = f"[screenshot taken by test-evo.py at {captured_at}]"
    line2 = f"{name} · {reason}"
    return line1, line2


def widths_for(records: list[FrameRecord]) -> dict[str, int]:
    timed = sum(1 for r in records if r.kind == KIND_TIMED)
    mandatory = sum(1 for r in records if r.kind == KIND_MANDATORY)
    return {
        KIND_TIMED: pad_width(timed),
        KIND_MANDATORY: pad_width(mandatory),
    }


def next_wake(
    now: float,
    *,
    mandatory_due: list[float],
    next_timed: float | None,
    deadline: float,
) -> float:
    candidates = [deadline]
    if mandatory_due:
        candidates.append(mandatory_due[0])
    if next_timed is not None:
        candidates.append(next_timed)
    wake = min(candidates)
    return max(0.0, wake - now)


@dataclass(frozen=True)
class Assertion:
    name: str
    status: str
    detail: str = ""

    def line(self) -> str:
        if self.detail:
            return f"[{self.status}] {self.name}: {self.detail}"
        return f"[{self.status}] {self.name}"


def strip_ansi(text: str) -> str:
    return ANSI_RE.sub("", text)


def visible_text(text: str) -> str:
    lines = [strip_ansi(line).rstrip() for line in text.splitlines()]
    while lines and not lines[0]:
        lines.pop(0)
    while lines and not lines[-1]:
        lines.pop()
    return "\n".join(lines)


def leading_spinner(text: str) -> str | None:
    vis = visible_text(text)
    if not vis:
        return None
    first = vis.splitlines()[0]
    if not first:
        return None
    glyph = first[0]
    if glyph in UNICODE_SPINNERS:
        return glyph
    if len(first) >= 3 and glyph in ASCII_SPINNERS and first[1:3] == "  ":
        return glyph
    return None


def assert_nonempty_vary(panes: dict[int, str]) -> Assertion:
    name = "mandatory-2..5 nonempty not identical"
    nonempty = {
        index: visible_text(panes.get(index, ""))
        for index in MANDATORY_COMPARE_INDEXES
        if visible_text(panes.get(index, ""))
    }
    if len(nonempty) < 2:
        return Assertion(name, STATUS_SKIP, f"{len(nonempty)} nonempty")
    unique = set(nonempty.values())
    if len(unique) == 1:
        return Assertion(
            name,
            STATUS_FAIL,
            f"{len(nonempty)} nonempty frames identical",
        )
    return Assertion(
        name,
        STATUS_OK,
        f"{len(unique)} distinct of {len(nonempty)} nonempty",
    )


def assert_spinner_moved(left: int, right: int, panes: dict[int, str]) -> Assertion:
    name = f"mandatory-{left} vs {right} spinner moved"
    left_glyph = leading_spinner(panes.get(left, ""))
    right_glyph = leading_spinner(panes.get(right, ""))
    missing = [
        str(index) for index, glyph in ((left, left_glyph), (right, right_glyph)) if glyph is None
    ]
    if missing:
        return Assertion(name, STATUS_SKIP, f"no spinner on {', '.join(missing)}")
    if left_glyph == right_glyph:
        return Assertion(name, STATUS_FAIL, f"both {left_glyph}")
    return Assertion(name, STATUS_OK, f"{left_glyph} → {right_glyph}")


def evaluate_mandatory_assertions(records: list[FrameRecord]) -> list[Assertion]:
    panes = {record.index: record.text for record in records if record.kind == KIND_MANDATORY}
    checks = [assert_nonempty_vary(panes)]
    checks.extend(assert_spinner_moved(left, right, panes) for left, right in SPINNER_PAIRS)
    return checks


def assertions_failed(checks: list[Assertion]) -> bool:
    return any(check.status == STATUS_FAIL for check in checks)
