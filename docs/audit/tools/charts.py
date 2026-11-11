"""SVG charts for the audit map: summary tiles, a package treemap and a heat map
of findings by kind. Standard library only; the output is deterministic, so
render.py --check can compare it byte for byte.

Each chart is drawn twice, for GitHub's light and dark themes, and README.md
picks between them with a <picture> element.
"""
import collections
from xml.sax.saxutils import escape

MONO = "ui-monospace, SFMono-Regular, Menlo, Consolas, monospace"
SANS = "-apple-system, BlinkMacSystemFont, 'Segoe UI', Helvetica, Arial, sans-serif"
# Advance widths used to decide whether a label fits: monospace is 0.6 em, and
# the sans estimate is deliberately generous so a label is dropped, not cut.
MONO_EM, SANS_EM = 0.6, 0.62

THEMES = {
    "light": {
        "fg": "#1f2328", "muted": "#59636e", "line": "#d1d9e0", "panel": "#f6f8fa",
        "tiers": {"Deep": ("#dafbe1", "#1a7f37"), "Moderate": ("#eaeef2", "#6e7781"),
                  "Light": ("#fff1c2", "#9a6700"), "Untouched": ("#ffebe9", "#cf222e")},
        "badge": "#bf8700", "badge_fg": "#ffffff", "heat": "9,105,218",
    },
    "dark": {
        "fg": "#e6edf3", "muted": "#9198a1", "line": "#3d444d", "panel": "#151b23",
        "tiers": {"Deep": ("#12321c", "#3fb950"), "Moderate": ("#262c36", "#8b949e"),
                  "Light": ("#3a2d0c", "#d29922"), "Untouched": ("#3d1418", "#f85149")},
        "badge": "#d29922", "badge_fg": "#0d1117", "heat": "56,139,253",
    },
}
TIERS = ["Deep", "Moderate", "Light", "Untouched"]


def f(v):
    """A coordinate, rounded so the output does not depend on float noise."""
    s = f"{v:.1f}"
    return s[:-2] if s.endswith(".0") else s


def text(x, y, s, size, color, family=SANS, anchor="start", weight=None):
    w = f' font-weight="{weight}"' if weight else ""
    return (f'<text x="{f(x)}" y="{f(y)}" font-family="{family}" font-size="{size}" fill="{color}" '
            f'text-anchor="{anchor}"{w}>{escape(str(s))}</text>')


def svg(width, height, label, body):
    return (f'<svg xmlns="http://www.w3.org/2000/svg" width="{width}" height="{height}" '
            f'viewBox="0 0 {width} {height}" role="img" aria-label="{escape(label)}">\n'
            f"<title>{escape(label)}</title>\n" + "\n".join(body) + "\n</svg>\n")


def fits(s, size, room, em=MONO_EM):
    return len(s) * size * em <= room


def summary(tiles, theme):
    """A row of tiles: (label, value, note) each."""
    t = THEMES[theme]
    width, height, gap = 960, 104, 12
    tw = (width - gap * (len(tiles) - 1)) / len(tiles)
    body = []
    for i, (label, value, note) in enumerate(tiles):
        x = i * (tw + gap)
        body.append(f'<rect x="{f(x + 0.5)}" y="0.5" width="{f(tw - 1)}" height="{height - 1}" rx="8" '
                    f'fill="{t["panel"]}" stroke="{t["line"]}"/>')
        body.append(text(x + 14, 26, label, 13, t["muted"]))
        body.append(text(x + 14, 62, value, 30, t["fg"], weight="600"))
        body.append(text(x + 14, 88, note, 11, t["muted"]))
    return svg(width, height, "Audit summary: " + "; ".join(f"{a} {b}" for a, b, _ in tiles), body)


def squarify(items, x, y, w, h):
    """Bruls, Huizing and van Wijk's squarified treemap over (value, data) items
    sorted by value, descending; returns (x, y, w, h, data) rectangles."""
    total = sum(v for v, _ in items)
    if total <= 0 or w <= 0 or h <= 0:
        return []
    scale = w * h / total
    rest = [(v * scale, d) for v, d in items]
    out = []

    def worst(row, side):
        s = sum(v for v, _ in row)
        return max(max(side * side * v / (s * s), s * s / (side * side * v)) for v, _ in row)

    while rest:
        side = min(w, h)
        row = [rest[0]]
        i = 1
        while i < len(rest) and worst(row + [rest[i]], side) <= worst(row, side):
            row.append(rest[i])
            i += 1
        rest = rest[i:]
        s = sum(v for v, _ in row)
        if w >= h:
            cw = s / h
            yy = y
            for v, d in row:
                out.append((x, yy, cw, v / cw, d))
                yy += v / cw
            x, w = x + cw, w - cw
        else:
            rh = s / w
            xx = x
            for v, d in row:
                out.append((xx, y, v / rh, rh, d))
                xx += v / rh
            y, h = y + rh, h - rh
    return out


def size(p):
    """A package's area in the treemap: the square root of its lines of code.

    Linear area spans a factor of 150 here (8,117 lines against 55), which
    leaves a third of the packages too small to carry their name; a static
    image has no tooltip to make up for it. The square root keeps the order
    and compresses the range to about 12."""
    return max(p["loc"], 1) ** 0.5


def treemap(packages, layers, theme):
    """Packages grouped by layer, area by the square root of lines of code,
    colored by depth, with a badge counting the items still missing."""
    t = THEMES[theme]
    width, height, top = 960, 800, 40
    body = []
    unlabeled = []
    # A short label must still name one package: "plan" is three of them.
    last = collections.Counter(p["package"].split("/")[-1] for p in packages)

    def labels(name):
        parts = name.split("/")
        short = parts[-1] if last[parts[-1]] == 1 else "/".join(parts[-2:])
        return [name] if short == name else [name, short]
    # Legend.
    x = 0
    for tier in TIERS:
        fill, stroke = t["tiers"][tier]
        body.append(f'<rect x="{x}" y="8" width="14" height="14" rx="3" fill="{fill}" stroke="{stroke}" '
                    f'stroke-width="1.5"/>')
        body.append(text(x + 20, 20, tier, 12, t["fg"]))
        x += 20 + len(tier) * 12 * SANS_EM + 22
    body.append(f'<circle cx="{f(x + 8)}" cy="15" r="8" fill="{t["badge"]}"/>')
    body.append(text(x + 8, 19, "2", 10, t["badge_fg"], anchor="middle", weight="700"))
    body.append(text(x + 22, 20, "items still missing", 12, t["fg"]))
    body.append(text(width, 20, "area: √ lines of code", 12, t["muted"], anchor="end"))

    groups = [(sum(size(p) for p in packages if p["layer"] == layer), layer) for layer in layers]
    groups = sorted((g for g in groups if g[0] > 0), key=lambda g: -g[0])
    for lx, ly, lw, lh, layer in squarify(groups, 0, top, width, height - top):
        lx, ly, lw, lh = lx + 2, ly + 2, lw - 4, lh - 4
        body.append(f'<rect x="{f(lx)}" y="{f(ly)}" width="{f(lw)}" height="{f(lh)}" rx="6" fill="none" '
                    f'stroke="{t["line"]}"/>')
        for label in (layer, layer.split(" ")[0]):
            if fits(label, 11, lw - 12, SANS_EM):
                body.append(text(lx + 6, ly + 14, label, 11, t["muted"], weight="600"))
                break
        rows = sorted((p for p in packages if p["layer"] == layer), key=lambda p: (-size(p), p["package"]))
        cells = squarify([(size(p), p) for p in rows], lx + 3, ly + 20, lw - 6, lh - 23)
        for cx, cy, cw, ch, p in cells:
            cx, cy, cw, ch = cx + 1, cy + 1, cw - 2, ch - 2
            fill, stroke = t["tiers"][p["depth"]]
            body.append(f'<g><title>{escape(p["package"])}: {p["depth"]}, {p["findings"]} findings, '
                        f'{p["gap_count"]} items missing</title>')
            body.append(f'<rect x="{f(cx)}" y="{f(cy)}" width="{f(max(cw, 0))}" height="{f(max(ch, 0))}" rx="3" '
                        f'fill="{fill}" stroke="{stroke}" stroke-width="1.2"/>')
            badge = p["gap_count"] > 0 and cw >= 26 and ch >= 22
            room = cw - 10 - (20 if badge else 0)
            placed = vertical = False
            for px in (11, 9):
                if placed or ch < px + 6:
                    break
                for label in labels(p["package"]):
                    if fits(label, px, room):
                        body.append(text(cx + 5, cy + px + 3, label, px, t["fg"], family=MONO))
                        placed = True
                        break
            # A tall, narrow cell reads its name bottom to top instead.
            for px in (11, 9):
                if placed or ch <= cw or cw < px + 4:
                    break
                for label in labels(p["package"]):
                    if fits(label, px, ch - 10 - (24 if badge else 0)):
                        x0, y0 = cx + cw / 2 + px * 0.35, cy + ch - 5
                        body.append(f'<text transform="translate({f(x0)},{f(y0)}) rotate(-90)" '
                                    f'font-family="{MONO}" font-size="{px}" fill="{t["fg"]}">'
                                    f"{escape(label)}</text>")
                        placed = vertical = True
                        break
            if not placed:
                unlabeled.append(p)
            if not vertical and ch >= 32 and fits(f"{p['findings']} findings", 10, cw - 10):
                body.append(text(cx + 5, cy + 28, f"{p['findings']} findings", 10, t["muted"], family=MONO))
            if badge:
                body.append(f'<circle cx="{f(cx + cw - 11)}" cy="{f(cy + 11)}" r="8" fill="{t["badge"]}"/>')
                body.append(text(cx + cw - 11, cy + 14.5, p["gap_count"], 10, t["badge_fg"], anchor="middle",
                                 weight="700"))
            body.append("</g>")
    # A cell too small for any label is named below the map, so the image
    # still identifies every package without a tooltip.
    if unlabeled:
        names = ", ".join(f"{p['package']} ({p['depth']}"
                          + (f", {p['gap_count']} missing" if p["gap_count"] else "") + ")"
                          for p in sorted(unlabeled, key=lambda p: p["package"]))
        line, lines = "Too small to label:", []
        for word in names.split(" "):
            if not fits(line + " " + word, 12, width, SANS_EM):
                lines.append(line)
                line = " "
            line += " " + word
        lines.append(line)
        for i, ln in enumerate(lines):
            body.append(text(0, height + 18 + 17 * i, ln.strip(), 12, t["muted"]))
        height += 10 + 17 * len(lines)
    return svg(width, height, "Treemap of the astrogo packages by layer, sized by lines of code, colored by "
               "audit depth, with a badge counting the items still missing", body)


def heatmap(packages, layers, kinds, theme):
    """Findings of each kind per package, grouped by layer."""
    t = THEMES[theme]
    label_w, col_w, row_h, head_h = 280, 64, 20, 28
    cols = kinds + [("findings", "Total")]
    width = label_w + col_w * len(cols)
    rows = []
    for layer in layers:
        members = sorted((p for p in packages if p["layer"] == layer), key=lambda p: p["package"])
        if members:
            rows.append(("layer", layer))
            rows += [("pkg", p) for p in members]
    height = head_h + row_h * len(rows) + 4
    top = max(p[k] for p in packages for k, _ in kinds) or 1
    body = []
    for i, (_, name) in enumerate(cols):
        body.append(text(label_w + col_w * (i + 0.5), 18, name, 11, t["muted"], anchor="middle", weight="600"))
    body.append(f'<line x1="0" y1="{head_h - 4}" x2="{width}" y2="{head_h - 4}" stroke="{t["line"]}"/>')
    y = head_h
    for kind, item in rows:
        if kind == "layer":
            body.append(text(0, y + 15, item, 11, t["muted"], weight="600"))
        else:
            body.append(text(0, y + 14, item["package"], 11, t["fg"], family=MONO))
            for i, (k, _) in enumerate(cols):
                n = item[k]
                x = label_w + col_w * i
                if k != "findings" and n:
                    alpha = 0.12 + 0.7 * (n / top) ** 0.5
                    body.append(f'<rect x="{f(x + 2)}" y="{f(y + 1)}" width="{col_w - 4}" height="{row_h - 2}" '
                                f'rx="3" fill="rgba({t["heat"]},{alpha:.2f})"/>')
                if n or k == "findings":
                    body.append(text(x + col_w / 2, y + 14, n, 11, t["fg"], family=MONO, anchor="middle",
                                     weight="600" if k == "findings" else None))
        y += row_h
    return svg(width, height, "Heat map of fixed findings by kind of defect for each package, grouped by layer",
               body)
