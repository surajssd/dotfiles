"""Box engine for Apple-style Manim explainer videos.

Copy this file next to a video module and import it with `from engine import *`.
Each video keeps its own copy, so an old video still renders after this file
changes.

The 3D look comes from a small projection engine (Cam, Box, render) instead of
ThreeDScene: Cairo's ThreeDScene sorts faces by their centers, which draws a
large board over the small chips that sit on it. Here every Box is sorted as a
whole with separating planes, which is exact while no two boxes intersect.
"""

from __future__ import annotations

import heapq

import numpy as np
from manim import *

FONT = "Helvetica Neue"
BG = "#000000"
TXT = "#f5f5f7"
GRAY = "#86868b"
LEADER = "#8e8e93"

# A dict rather than names like BLUE, which would replace Manim's own colors.
ACCENT = dict(green="#8fd400", blue="#2997ff", purple="#bf5af2", orange="#ff9f0a", cyan="#64d2ff", neutral="#8e8e93")

config.background_color = BG

EASE = rate_functions.ease_in_out_cubic


# ---------------------------------------------------------------- colors ---


def _rgb(c):
    return np.array(ManimColor(c).to_rgb())


def _hex(v):
    return ManimColor.from_rgb(np.clip(v, 0, 1))


def shade(c, k):
    return _hex(_rgb(c) * k)


def mix(a, b, t):
    t = float(np.clip(t, 0, 1))
    return _hex(_rgb(a) * (1 - t) + _rgb(b) * t)


# Brightness per face normal for a key light at the front-left, above.
FACE_LIGHT = {"top": 1.0, "-y": 0.62, "+y": 0.40, "-x": 0.52, "+x": 0.44, "bot": 0.28}


# ---------------------------------------------------------------- camera ---


class Cam:
    """Perspective camera orbiting `target`; theta is azimuth, phi elevation."""

    def __init__(self, theta, phi, dist, zoom, target, shift):
        th, ph = np.radians(theta), np.radians(phi)
        self.theta = theta
        self.target = np.asarray(target, float)
        self.pos = self.target + dist * np.array(
            [np.cos(ph) * np.cos(th), np.cos(ph) * np.sin(th), np.sin(ph)]
        )
        f = self.target - self.pos
        self.f = f / np.linalg.norm(f)
        r = np.cross(self.f, [0.0, 0.0, 1.0])
        self.r = r / np.linalg.norm(r)
        self.u = np.cross(self.r, self.f)
        self.dist, self.zoom = dist, zoom
        self.shift = np.array([shift[0], shift[1]])

    def proj(self, pts):
        pts = np.asarray(pts, float).reshape(-1, 3)
        v = pts - self.pos
        k = self.zoom * self.dist / (v @ self.f)
        out = np.zeros((len(pts), 3))
        out[:, 0] = (v @ self.r) * k + self.shift[0]
        out[:, 1] = (v @ self.u) * k + self.shift[1]
        return out

    def p1(self, p):
        return self.proj(p)[0]

    def depth(self, pts):
        return (np.asarray(pts, float).reshape(-1, 3) - self.pos) @ self.f

    def screen_dir(self, at, world_dir):
        a, b = self.proj(np.array([at, np.asarray(at) + np.asarray(world_dir) * 0.5]))
        d = b - a
        n = np.linalg.norm(d)
        return d / n if n > 1e-9 else RIGHT


# ------------------------------------------------------------- primitives ---


def poly(pts, fill, op, sc=None, sw=0.0, so=1.0, sheen=None):
    m = VMobject()
    m.set_points_as_corners(np.vstack([pts, pts[:1]]))
    m.set_fill(fill, op)
    if sw > 0:
        m.set_stroke(sc if sc is not None else fill, sw, so)
    else:
        m.set_stroke(width=0)
    if sheen is not None:
        m.set_sheen_direction(sheen)
    return m


def polyline(pts, color, width, op):
    m = VMobject()
    m.set_points_as_corners(pts)
    m.set_fill(opacity=0)
    m.set_stroke(color, width, op)
    m.joint_type = LineJointType.ROUND
    m.cap_style = CapStyleType.ROUND
    return m


def glow_line(pts, color, width, op, layers=((7, 0.06), (3.2, 0.16), (1, 1.0))):
    if op <= 0.004 or len(pts) < 2:
        return []
    return [polyline(pts, color, width * w, op * o) for w, o in layers]


def glow_dot(p, color, r, op):
    if op <= 0.004:
        return []
    out = []
    for k, o in ((3.2, 0.10), (1.9, 0.22), (1.0, 1.0)):
        c = Circle(radius=r * k, stroke_width=0)
        c.set_fill(color if k > 1 else mix(color, WHITE, 0.55), op * o)
        c.move_to(p)
        out.append(c)
    return out


def bezier3(a, b, lift, n=24):
    a, b = np.asarray(a, float), np.asarray(b, float)
    c = (a + b) / 2 + np.array([0, 0, lift])
    t = np.linspace(0, 1, n)[:, None]
    return (1 - t) ** 2 * a + 2 * (1 - t) * t * c + t**2 * b


def path_point(path, s):
    """Point at fraction s along a polyline (by vertex index, good enough here)."""
    s = float(np.clip(s, 0, 1)) * (len(path) - 1)
    i = min(int(s), len(path) - 2)
    f = s - i
    return path[i] * (1 - f) + path[i + 1] * f


def path_upto(path, s):
    s = float(np.clip(s, 0, 1))
    if s <= 0:
        return path[:1]
    k = s * (len(path) - 1)
    i = int(k)
    pts = list(path[: i + 1])
    if i < len(path) - 1:
        pts.append(path_point(path, s))
    return np.array(pts)


# ------------------------------------------------------------------ boxes ---


class Box:
    """Axis-aligned box. `c` is the center of its bottom face, `size` is (w, d, h)."""

    def __init__(
        self,
        name,
        c,
        size,
        color,
        group,
        *,
        parent=None,
        offset=None,
        alpha=None,
        geom=None,
        deco=None,
        gloss=0.10,
        metal=False,
        edge=None,
        edge_op=0.45,
    ):
        self.name = name
        self.c = np.array(c, float)
        self.size = np.array(size, float)
        self.color = color
        self.group = group
        self.parent = parent
        self.offset = offset
        self.alpha_fn = alpha
        self.geom = geom
        self.deco = deco
        self.metal = metal
        self.edge = edge or shade(color, 1.9)
        self.edge_op = edge_op
        base = _rgb(color)
        self.cols = {k: _hex(base * v) for k, v in FACE_LIGHT.items()}
        self.top_hi = _hex(base * (1 + gloss * (2.2 if metal else 1)))
        self.top_lo = _hex(base * (1 - gloss))
        self.lo = self.hi = None
        self.a = 0.0

    def update(self, P):
        if self.geom:
            c, size = self.geom(P)
            c, size = np.asarray(c, float), np.asarray(size, float)
        else:
            c, size = self.c, self.size
            if self.offset:
                c = c + np.asarray(self.offset(P), float)
        self.lo = np.array([c[0] - size[0] / 2, c[1] - size[1] / 2, c[2]])
        self.hi = np.array([c[0] + size[0] / 2, c[1] + size[1] / 2, c[2] + size[2]])
        a = P["reveal"] * P.get("a_" + self.group, 1.0)
        if self.alpha_fn:
            a *= self.alpha_fn(P)
        self.a = float(np.clip(a, 0, 1))

    @property
    def top_center(self):
        return np.array([(self.lo[0] + self.hi[0]) / 2, (self.lo[1] + self.hi[1]) / 2, self.hi[2]])

    def top_uv(self, u, v, dz=0.0):
        return np.array(
            [
                self.lo[0] + u * (self.hi[0] - self.lo[0]),
                self.lo[1] + v * (self.hi[1] - self.lo[1]),
                self.hi[2] + dz,
            ]
        )

    def faces(self, cam):
        (x0, y0, z0), (x1, y1, z1) = self.lo, self.hi
        p = cam.pos
        out = []
        if p[1] < y0:
            out.append(("-y", [(x0, y0, z0), (x1, y0, z0), (x1, y0, z1), (x0, y0, z1)]))
        if p[1] > y1:
            out.append(("+y", [(x0, y1, z0), (x1, y1, z0), (x1, y1, z1), (x0, y1, z1)]))
        if p[0] < x0:
            out.append(("-x", [(x0, y0, z0), (x0, y1, z0), (x0, y1, z1), (x0, y0, z1)]))
        if p[0] > x1:
            out.append(("+x", [(x1, y0, z0), (x1, y1, z0), (x1, y1, z1), (x1, y0, z1)]))
        if p[2] < z0:
            out.append(("bot", [(x0, y0, z0), (x1, y0, z0), (x1, y1, z0), (x0, y1, z0)]))
        if p[2] > z1:
            out.append(("top", [(x0, y0, z1), (x1, y0, z1), (x1, y1, z1), (x0, y1, z1)]))
        return out


class Face:
    """Projects (u, v) coordinates on a box's top face to the screen."""

    def __init__(self, box, cam):
        self.b, self.cam = box, cam

    def pts(self, uv, dz=0.0):
        uv = np.asarray(uv, float)
        b = self.b
        w = np.stack(
            [
                b.lo[0] + uv[:, 0] * (b.hi[0] - b.lo[0]),
                b.lo[1] + uv[:, 1] * (b.hi[1] - b.lo[1]),
                np.full(len(uv), b.hi[2] + dz),
            ],
            axis=1,
        )
        return self.cam.proj(w)

    def rect(self, u0, v0, u1, v1, fill, op, sc=None, sw=0.0, so=1.0, sheen=None, dz=0.0):
        p = self.pts([(u0, v0), (u1, v0), (u1, v1), (u0, v1)], dz)
        return poly(p, fill, op, sc, sw, so, sheen)

    def screen_width(self):
        a, b = self.pts([(0, 0.5), (1, 0.5)])
        return np.linalg.norm(b - a)


def render(boxes, cam, P, children):
    act = [b for b in boxes if b.a > 0.004]
    n = len(act)
    if n == 0:
        return []
    lo = np.array([b.lo for b in act])
    hi = np.array([b.hi for b in act])
    idx = np.array([[0, 0, 0], [1, 0, 0], [0, 1, 0], [1, 1, 0], [0, 0, 1], [1, 0, 1], [0, 1, 1], [1, 1, 1]])
    corners = lo[:, None, :] + idx[None, :, :] * (hi - lo)[:, None, :]
    sp = cam.proj(corners.reshape(-1, 3)).reshape(n, 8, 3)
    smin, smax = sp.min(axis=1), sp.max(axis=1)
    dep = cam.depth((lo + hi) / 2)

    ov = (
        (smin[:, 0][:, None] < smax[:, 0][None, :])
        & (smin[:, 0][None, :] < smax[:, 0][:, None])
        & (smin[:, 1][:, None] < smax[:, 1][None, :])
        & (smin[:, 1][None, :] < smax[:, 1][:, None])
    )
    np.fill_diagonal(ov, False)
    before = np.zeros((n, n), bool)
    decided = np.zeros((n, n), bool)
    eps = 1e-5
    for ax in (2, 0, 1):
        low = hi[:, ax][:, None] <= lo[:, ax][None, :] + eps
        m = low & ~decided & ov
        cam_beyond = np.broadcast_to(cam.pos[ax] > hi[:, ax][:, None], (n, n))
        before |= m & cam_beyond
        before |= (m & ~cam_beyond).T
        decided |= m | m.T
    rest = ov & ~decided
    before |= rest & (dep[:, None] > dep[None, :])

    indeg = before.sum(axis=0)
    heap = [(-dep[i], i) for i in range(n) if indeg[i] == 0]
    heapq.heapify(heap)
    order, seen = [], np.zeros(n, bool)
    while heap:
        _, i = heapq.heappop(heap)
        order.append(i)
        seen[i] = True
        for j in np.nonzero(before[i])[0]:
            indeg[j] -= 1
            if indeg[j] == 0:
                heapq.heappush(heap, (-dep[j], j))
    if len(order) < n:
        order += sorted(np.nonzero(~seen)[0], key=lambda i: -dep[i])

    glint = cam.screen_dir(
        cam.target, (np.cos(np.radians(cam.theta * 1.7 + 30)), np.sin(np.radians(cam.theta * 1.7 + 30)), 0)
    )
    out = []
    for i in order:
        b = act[i]
        for name, quad in b.faces(cam):
            pts = cam.proj(np.array(quad))
            if name == "top":
                out.append(poly(pts, [b.top_hi, b.top_lo], b.a, b.edge, 0.7, b.edge_op * b.a, sheen=glint))
            else:
                col = b.cols[name]
                out.append(poly(pts, col, b.a, col, 0.5, b.a))
        if cam.pos[2] > b.hi[2]:
            face = Face(b, cam)
            for ch in children.get(b.name, ()):
                out += shadow(face, ch)
            if b.deco:
                out += b.deco(b, face, P)
    return out


def shadow(face, ch):
    gap = ch.lo[2] - face.b.hi[2]
    if ch.a < 0.01 or gap < 0.015:
        return []
    k = ch.a * float(np.clip(gap / 0.15, 0, 1)) * np.exp(-gap / 2.2)
    b = face.b
    w, d = b.hi[0] - b.lo[0], b.hi[1] - b.lo[1]
    sx, sy = 0.10 * gap, 0.16 * gap
    out = []
    for grow, o in ((0.10 + 0.22 * gap, 0.10), (0.05 + 0.10 * gap, 0.14), (0.0, 0.20)):
        u0 = (ch.lo[0] - grow + sx - b.lo[0]) / w
        u1 = (ch.hi[0] + grow + sx - b.lo[0]) / w
        v0 = (ch.lo[1] - grow + sy - b.lo[1]) / d
        v1 = (ch.hi[1] + grow + sy - b.lo[1]) / d
        out.append(face.rect(u0, v0, u1, v1, BLACK, o * k * 2.2))
    return out


class Stage(VGroup):
    """Rebuilt from scratch every frame by `fn`."""

    def __init__(self, fn):
        super().__init__()
        self.fn = fn
        self.refresh()
        self.add_updater(lambda m: m.refresh())

    def refresh(self):
        self.submobjects = list(self.fn())
        return self


# ------------------------------------------------------------- typography ---


def eyebrow(s, color):
    return MarkupText(f'<span letter_spacing="1800">{s}</span>', font=FONT, weight=BOLD, font_size=17, color=color)


def headline(lines, size=46, color=TXT, gradient=None):
    rows = []
    for ln in lines.split("\n"):
        kw = dict(gradient=gradient) if gradient else dict(color=color)
        rows.append(Text(ln, font=FONT, weight=BOLD, font_size=size, **kw))
    return VGroup(*rows).arrange(DOWN, aligned_edge=LEFT, buff=0.14)


def body(lines, size=21, color=GRAY):
    rows = [Text(ln, font=FONT, font_size=size, color=color) for ln in lines.split("\n")]
    return VGroup(*rows).arrange(DOWN, aligned_edge=LEFT, buff=0.11)


def scrim(width=7.6):
    r = Rectangle(width=width, height=config.frame_height + 0.2, stroke_width=0)
    r.set_fill([BLACK, BLACK, BLACK], opacity=[0.92, 0.75, 0.0])
    r.set_sheen_direction(RIGHT)
    r.move_to(np.array([-config.frame_width / 2, 0, 0]), aligned_edge=LEFT)
    return r


def text_block(eb, hl, bd, color, corner=(-6.55, 2.95), hl_gradient=None, with_scrim=False):
    parts = [eyebrow(eb, color), headline(hl, gradient=hl_gradient)]
    if bd:
        parts.append(body(bd))
    g = VGroup(*parts).arrange(DOWN, aligned_edge=LEFT, buff=0.28)
    g[1].shift(UP * 0.08)
    g.move_to(np.array([corner[0], corner[1], 0]), aligned_edge=UL)
    if with_scrim:
        return VGroup(scrim(), g)
    return g


def hbars(rows, max_value, width=7.6, x=-2.2, top=1.75, gap=0.82, bar_height=0.34):
    """Horizontal bars on a linear scale.

    Each row is (name, note, value, value_text, color). Returns one
    VGroup(label, bar, value) per row; Base.show_bars animates them.
    """
    out = []
    for i, (name, note, value, value_text, color) in enumerate(rows):
        y = top - i * gap
        lab = Text(name, font=FONT, weight=BOLD, font_size=22, color=TXT)
        if note:
            lab = VGroup(lab, Text(note, font=FONT, font_size=15, color=GRAY)).arrange(DOWN, aligned_edge=RIGHT, buff=0.06)
        lab.move_to(np.array([x - 0.3, y, 0]), aligned_edge=RIGHT)
        bar = Rectangle(width=max(width * value / max_value, 0.04), height=bar_height, stroke_width=0).set_fill(color, 1)
        bar.move_to(np.array([x, y, 0]), aligned_edge=LEFT)
        num = Text(value_text, font=FONT, weight=BOLD, font_size=20, color=color).next_to(bar, RIGHT, buff=0.15)
        out.append(VGroup(lab, bar, num))
    return out


# ------------------------------------------------------------------ scene ---

ENGINE_DEFAULTS = dict(theta=-118.0, phi=36.0, dist=34.0, zoom=0.6, tx=0.0, ty=0.0, tz=0.0, sx=0.0, sy=0.0, reveal=1.0)


class Base(Scene):
    """A Scene whose Stage redraws the boxes from world() on every frame.

    A video subclasses Base once to set DEFAULTS (every tracker it animates,
    including one a_<group> opacity per box group), world(), and overlays().
    Each scene subclasses that class and sets START, the tracker values it
    begins with.
    """

    DEFAULTS: dict = {}
    START: dict = {}

    def world(self):
        return []

    def overlays(self, B, cam, P):
        return []

    def setup(self):
        defaults = {**ENGINE_DEFAULTS, **self.DEFAULTS}
        unknown = sorted(set(self.START) - set(defaults))
        if unknown:
            raise ValueError(f"{type(self).__name__}.START sets keys that DEFAULTS lacks: {unknown}")
        P = {**defaults, **self.START}
        self.trk = {k: ValueTracker(v) for k, v in P.items()}
        self.clock = ValueTracker(0.0)
        self.clock.add_updater(lambda m, dt: m.increment_value(dt))
        self.boxes = list(self.world())
        self.B = {b.name: b for b in self.boxes}
        if len(self.B) != len(self.boxes):
            raise ValueError("world() returned two boxes with the same name")
        self.children = {}
        for b in self.boxes:
            if b.parent:
                self.children.setdefault(b.parent, []).append(b)
        self.stage = Stage(self.draw)
        self.add(self.clock, self.stage)

    def get_moving_and_static_mobjects(self, animations):
        # The stock version flattens families once per play(), which keeps the
        # Stage's polygons from the first frame on screen as a stale copy.
        return list(self.mobjects), []

    def vals(self):
        P = {k: t.get_value() for k, t in self.trk.items()}
        P["t"] = self.clock.get_value()
        return P

    def cam(self, P=None):
        P = P or self.vals()
        return Cam(P["theta"], P["phi"], P["dist"], P["zoom"], (P["tx"], P["ty"], P["tz"]), (P["sx"], P["sy"]))

    def draw(self):
        P = self.vals()
        cam = self.cam(P)
        for b in self.boxes:
            b.update(P)
        return render(self.boxes, cam, P, self.children) + list(self.overlays(self.B, cam, P))

    def to(self, *extra, run_time=3.0, rate=EASE, **kw):
        anims = [self.trk[k].animate.set_value(v) for k, v in kw.items()]
        self.play(*anims, *extra, run_time=run_time, rate_func=rate)

    def snap(self):
        P = self.vals()
        for b in self.boxes:
            b.update(P)
        return self.cam(P)

    def callout(self, world_pt, title, sub=None, to=(1.2, 0.6), color=TXT, size=24, up=False):
        cam = self.snap()
        p = cam.p1(world_pt)
        end = p + np.array([to[0], to[1], 0])
        dot = Dot(p, radius=0.045, color=WHITE)
        ring = Circle(radius=0.09, stroke_color=WHITE, stroke_width=1.2).move_to(p)
        line = Line(p, end, stroke_width=1.3, color=LEADER)
        lab = Text(title, font=FONT, weight=BOLD, font_size=size, color=color)
        if sub:
            rows = [lab] + [Text(ln, font=FONT, font_size=size - 6, color=GRAY) for ln in sub.split("\n")]
            edge = ORIGIN if up else (LEFT if to[0] >= 0 else RIGHT)
            lab = VGroup(*rows).arrange(DOWN, aligned_edge=edge, buff=0.08)
        if up:
            lab.next_to(end, UP, buff=0.12)
        elif to[1] < -0.3:
            lab.move_to(end + DOWN * 0.1, aligned_edge=UR if to[0] < 0 else UL)
        elif to[0] >= 0:
            lab.next_to(end, RIGHT, buff=0.14)
        else:
            lab.next_to(end, LEFT, buff=0.14)
        return VGroup(line, ring, dot, lab)

    def show(self, *callouts, run_time=1.0, lag=0.25):
        anims = []
        for c in callouts:
            line, ring, dot, lab = c
            anims.append(Succession(AnimationGroup(FadeIn(dot), FadeIn(ring, scale=0.5)), Create(line, run_time=0.5), FadeIn(lab, shift=0.1 * UP)))
        self.play(LaggedStart(*anims, lag_ratio=lag), run_time=run_time + lag * len(callouts))

    def fade(self, *mobs, run_time=0.8):
        self.play(*[FadeOut(m) for m in mobs], run_time=run_time)

    def show_bars(self, rows, per_row=0.75):
        for lab, bar, num in rows:
            self.play(FadeIn(lab, shift=0.1 * RIGHT), GrowFromEdge(bar, LEFT), FadeIn(num), run_time=per_row)
