"""Check that a live pty stream tells the truth: rows settle forward, and the
final screen equals the Plain rows of an identical run.

Rows are read the way a person reads them: a glyph, a name (up to the first
run of two spaces), then an optional reason. Spinners and ``○`` waiting rows
are in-progress; the glyphs in SETTLED never go back.

Order is one rule for both renderers (CONTRACT section 8, "Finish order"): a row
never prints above the block that explains it, so a row that ran After a block
prints below it, and rows held for a block print in the order they were released.
The one place Plain and the Human final screen differ is a block Plain streamed
(``◐ name``, members as they settle, closing verdict): they agree on its settled
form, the verdict row first, with the members compared as a set because Plain
lists them in settlement order and the screen in declaration order. Every other
row compares in order.
"""

from __future__ import annotations

import re
from collections.abc import Iterable, Sequence
from typing import NamedTuple

import ptystream_frames as stream

SETTLED = frozenset("✓✗-!■")
FINAL = SETTLED | {"○"}
PROGRESS = "◐"
INDENT = 3
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


class Row(NamedTuple):
    """One final row: its depth, normalized text, name, and whether Plain streamed it as a block."""

    depth: int
    text: str
    name: str
    streamed: bool = False


def _rows(raw: Iterable[str]) -> list[Row]:
    """The final rows, run summaries and ``◐`` rows of raw, with depth and name."""
    rows = []
    for row in raw:
        text = " ".join(row.split())
        glyph_row = text[1:2] == " " and (text[:1] in FINAL or text[:1] == PROGRESS)
        if not glyph_row and not text.startswith(SUMMARY_PREFIX):
            continue
        depth = (len(row) - len(row.lstrip(" "))) // INDENT
        parts = split_row(row)
        rows.append(Row(depth, DURATION_RE.sub("", text), parts[1] if parts else text))
    return rows


class _Node:
    def __init__(self, row: Row, path: tuple[str, ...]) -> None:
        self.row, self.path = row, path
        self.children: list[_Node] = []
        self.open = False


def _tree(rows: Sequence[Row]) -> list[_Node]:
    """Rows nested by depth, each streamed block in its settled form.

    Plain streams a block that outlasts its grace as ``◐ name``, its members as
    they settle, then the closing verdict; the Human final screen shows the
    verdict first. A block whose members another block's lines interrupted heads
    again (``◐ name`` at the same depth under the same parent), so every header
    of one name under one parent is one block: it takes the place of its first
    header, the members of all its stretches, and the closing verdict as its row.
    A ``◐`` row no verdict closes is a heartbeat for a long Task and is dropped,
    its members, if any, staying where they were. Each node knows its path of
    names from the root, so a member's parent is part of what a row is.
    """
    roots: list[_Node] = []
    stack: list[_Node] = []
    blocks: dict[tuple[tuple[str, ...], str], _Node] = {}
    for row in rows:
        del stack[row.depth :]
        parent = stack[-1] if stack else None
        path = parent.path if parent else ()
        key = (path, row.name)
        glyph = row.text[:1]
        node = blocks.get(key) if glyph == PROGRESS or glyph in FINAL else None
        if node is None:
            node = _Node(row, path + (row.name,))
            node.open = glyph == PROGRESS
            (parent.children if parent else roots).append(node)
            if node.open:
                blocks[key] = node
        elif glyph in FINAL:
            node.row, node.open = row._replace(streamed=True), False
            del blocks[key]
        stack.append(node)
    return _without_heartbeats(roots)


def _without_heartbeats(nodes: Iterable[_Node]) -> list[_Node]:
    """The nodes without the ``◐`` rows no verdict closed, their members kept in place."""
    kept: list[_Node] = []
    for node in nodes:
        node.children = _without_heartbeats(node.children)
        if node.open:
            kept.extend(node.children)
        else:
            kept.append(node)
    return kept


def _paths(nodes: Iterable[_Node]) -> set[tuple[str, ...]]:
    found: set[tuple[str, ...]] = set()
    for node in nodes:
        if node.row.streamed:
            found.add(node.path)
        found |= _paths(node.children)
    return found


def _flatten(nodes: Iterable[_Node], streamed: set[tuple[str, ...]]) -> list[str]:
    """Rows in order, indented by depth; a streamed block's members sort by their text."""
    flat: list[str] = []
    for node in nodes:
        flat.append(INDENT * " " * node.row.depth + node.row.text)
        members = node.children
        if node.path in streamed:
            members = sorted(members, key=lambda child: child.row.text)
        flat.extend(_flatten(members, streamed))
    return flat


def streamed_blocks(plain_rows: Iterable[str]) -> set[tuple[str, ...]]:
    """The paths of the containers Plain wrote as a streamed block."""
    return _paths(_tree(_rows(plain_rows)))


def normalize_rows(
    rows: Iterable[str], streamed: set[tuple[str, ...]] | None = None
) -> list[str]:
    """Final rows (glyph, name, reason) and run summaries in screen order, durations dropped.

    The agreed form of a streamed block (CONTRACT section 8) is its settled form:
    the closing verdict row first, then its members. Plain lists the members as they
    settled and the Human final screen in declaration order, so the members of a
    block named in ``streamed`` compare sorted. Rows keep their depth as indentation.
    Everything else compares in order, so a row printed above its cause still fails.
    """
    nodes = _tree(_rows(rows))
    return _flatten(nodes, streamed if streamed is not None else _paths(nodes))


def check_final_matches_plain(
    final: stream.Frame, screen_exit: int, plain_rows: Iterable[str], plain_exit: int
) -> None:
    """The last pty frame holds the Plain run's final rows, in order, and the exit codes agree.

    The sorted comparison runs first so a missing or extra row gets its specific
    diagnostic. Only when the row sets match is the order checked, row by row. The
    rows are walked as a tree (a block's members sit under their own header), so a
    late member Plain wrote under another block's lines is a different row order.
    """
    if screen_exit != plain_exit:
        raise TruthError(f"pty run exit {screen_exit}, Plain run exit {plain_exit}")
    plain_rows = list(plain_rows)
    streamed = streamed_blocks(plain_rows)
    screen, plain = (
        normalize_rows(final.rows, streamed),
        normalize_rows(plain_rows, streamed),
    )
    if sorted(screen) != sorted(plain):
        missing = [row for row in plain if row not in screen]
        extra = [row for row in screen if row not in plain]
        raise TruthError(
            f"final screen differs from Plain: missing {missing[:3]}, extra {extra[:3]}"
            f" ({len(screen)} screen rows, {len(plain)} Plain rows)"
        )
    for index, (got, want) in enumerate(zip(screen, plain)):
        if got != want:
            raise TruthError(
                f"final screen order differs from Plain at row {index}: "
                f"screen {got!r}, Plain {want!r}"
            )
