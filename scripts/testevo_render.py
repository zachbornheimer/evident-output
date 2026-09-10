"""Render tmux capture-pane (-e) text to a PNG using a simple SGR parser."""

from __future__ import annotations

from pathlib import Path

from PIL import Image, ImageDraw, ImageFont

FONT_SIZE = 14
CELL_WIDTH = 9
CELL_HEIGHT = 18
PAD_X = 4
PAD_Y = 4

MENLO_PATH = "/System/Library/Fonts/Menlo.ttc"
COURIER_PATH = "/System/Library/Fonts/Supplemental/Courier New.ttf"

# Standard 16-color ANSI (normal then bright).
ANSI_COLORS: tuple[tuple[int, int, int], ...] = (
    (0, 0, 0),
    (205, 49, 49),
    (13, 188, 121),
    (229, 229, 16),
    (36, 114, 200),
    (188, 63, 188),
    (17, 168, 205),
    (204, 204, 204),
    (118, 118, 118),
    (241, 76, 76),
    (35, 209, 139),
    (245, 245, 67),
    (59, 142, 234),
    (214, 112, 214),
    (41, 184, 219),
    (255, 255, 255),
)

PAPER_FG = (0, 0, 0)
PAPER_BG = (255, 255, 255)
FOOTER_FG = (70, 70, 70)
FOOTER_BG = (236, 236, 236)
FOOTER_HEIGHT = 36
# Light-on-dark terminal ink is unreadable on paper; remap those.
INK_LUMA_MAX = 160.0

DEFAULT_FG = PAPER_FG
DEFAULT_BG = PAPER_BG


def load_font(size: int = FONT_SIZE) -> ImageFont.ImageFont | ImageFont.FreeTypeFont:
    try:
        return ImageFont.truetype(MENLO_PATH, size=size, index=0)
    except OSError:
        pass
    try:
        return ImageFont.truetype(COURIER_PATH, size=size)
    except OSError:
        return ImageFont.load_default()


Color = tuple[int, int, int]
Cell = tuple[str, Color, Color, bool]


def _apply_sgr(
    params: list[int], bold: bool, dim: bool, fg: Color, bg: Color
) -> tuple[bool, bool, Color, Color]:
    if not params:
        params = [0]
    for code in params:
        if code == 0:
            bold, dim = False, False
            fg, bg = DEFAULT_FG, DEFAULT_BG
        elif code == 1:
            bold = True
        elif code == 2:
            dim = True
        elif code == 22:
            bold, dim = False, False
        elif code == 39:
            fg = DEFAULT_FG
        elif code == 49:
            bg = DEFAULT_BG
        elif 30 <= code <= 37:
            fg = ANSI_COLORS[code - 30]
        elif 90 <= code <= 97:
            fg = ANSI_COLORS[code - 90 + 8]
        elif 40 <= code <= 47:
            bg = ANSI_COLORS[code - 40]
        elif 100 <= code <= 107:
            bg = ANSI_COLORS[code - 100 + 8]
    return bold, dim, fg, bg


def _luma(color: Color) -> float:
    return 0.299 * color[0] + 0.587 * color[1] + 0.114 * color[2]


def _paper_fg(color: Color) -> Color:
    if _luma(color) > INK_LUMA_MAX:
        return PAPER_FG
    return color


def _dim_color(color: Color) -> Color:
    if color == PAPER_FG:
        return (110, 110, 110)
    return (
        (color[0] + PAPER_BG[0]) // 2,
        (color[1] + PAPER_BG[1]) // 2,
        (color[2] + PAPER_BG[2]) // 2,
    )


def _parse_line(line: str) -> list[Cell]:
    """Return cells as (char, fg, bg, bold)."""
    cells: list[Cell] = []
    bold = False
    dim = False
    fg: Color = DEFAULT_FG
    bg: Color = DEFAULT_BG
    i = 0
    while i < len(line):
        ch = line[i]
        if ch == "\x1b" and i + 1 < len(line) and line[i + 1] == "[":
            i += 2
            start = i
            while i < len(line) and not ("@" <= line[i] <= "~"):
                i += 1
            body = line[start:i]
            final = line[i] if i < len(line) else ""
            i += 1
            if final == "m":
                params: list[int] = []
                if body == "":
                    params = [0]
                else:
                    for part in body.split(";"):
                        if part.isdigit():
                            params.append(int(part))
                        elif part == "":
                            params.append(0)
                bold, dim, fg, bg = _apply_sgr(params, bold, dim, fg, bg)
            continue
        if ch in ("\r", "\0"):
            i += 1
            continue
        ink = _paper_fg(fg)
        draw_fg = _dim_color(ink) if dim else ink
        cells.append((ch, draw_fg, PAPER_BG, bold))
        i += 1
    return cells


def render_ansi_to_png(
    text: str,
    dest: Path,
    cols: int,
    rows: int,
    *,
    captured_at: str | None = None,
    name: str | None = None,
    reason: str | None = None,
) -> None:
    """Render capture-pane text to dest as a PNG of cols x rows cells on paper."""
    font = load_font()
    footer_font = load_font(size=11)
    width = PAD_X * 2 + cols * CELL_WIDTH
    body_height = PAD_Y * 2 + rows * CELL_HEIGHT
    height = body_height + FOOTER_HEIGHT
    image = Image.new("RGB", (width, height), PAPER_BG)
    draw = ImageDraw.Draw(image)

    raw_lines = text.split("\n")
    # tmux may omit a trailing newline; pad/truncate to rows.
    lines = (raw_lines + [""] * rows)[:rows]

    for row_idx, line in enumerate(lines):
        cells = _parse_line(line.rstrip("\r"))
        for col_idx in range(cols):
            if col_idx < len(cells):
                char, fg, _bg, bold = cells[col_idx]
            else:
                char, fg, bold = " ", PAPER_FG, False
            x = PAD_X + col_idx * CELL_WIDTH
            y = PAD_Y + row_idx * CELL_HEIGHT
            if char != " ":
                draw.text((x, y), char, fill=fg, font=font)
                if bold:
                    draw.text((x + 1, y), char, fill=fg, font=font)

    draw.rectangle([0, body_height, width, height], fill=FOOTER_BG)
    if captured_at:
        line1 = f"[screenshot taken by test-evo.py at {captured_at}]"
        draw.text((PAD_X, body_height + 3), line1, fill=FOOTER_FG, font=footer_font)
        if name or reason:
            bits = [part for part in (name, reason) if part]
            line2 = " · ".join(bits)
            draw.text((PAD_X, body_height + 18), line2, fill=FOOTER_FG, font=footer_font)

    image.save(dest, format="PNG")
