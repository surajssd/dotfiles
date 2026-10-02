"""Starting point for an explainer video built on engine.py.

Copy this file and engine.py into a new folder, then replace the world, the
overlay, the poses, the text, and the scenes. The three scenes show the main
techniques: a camera reveal, callouts, an exploded view, a stack that splits
into layers, a glowing link with pulses, and a bar chart.
"""

from __future__ import annotations

import numpy as np
from manim import *

from engine import *

BOARD_Z = 0.14
SUB_Z = BOARD_Z + 0.08
STACK_H = 0.09
SPLIT_STACK = (-1.65, -0.25)
STACK_LAYERS = 5


def lvl2(P):
    return np.array([0, 0, 1.2 * P["explode"]])


def lvl3(P):
    return np.array([0, 0, 2.2 * P["explode"]])


def die_deco(b, F, P):
    sheen = F.cam.screen_dir(b.top_center, (1, 1, 0))
    out = [F.rect(0.03, 0.03, 0.97, 0.97, ["#3a7c86", "#4b3f8f", "#8c6c3c"], 0.3 * b.a, sheen=sheen)]
    if F.screen_width() > 0.8:
        for col in range(3):
            for row in range(2):
                u0, v0 = 0.06 + col * 0.30, 0.08 + row * 0.44
                out.append(F.rect(u0, v0, u0 + 0.27, v0 + 0.38, BLACK, 0.0, "#7f8aa8", 0.6, 0.35 * b.a))
    return out


def mem_deco(b, F, P):
    g = P["mem_glow"] * b.a
    if g < 0.01:
        return []
    return [F.rect(0, 0, 1, 1, ACCENT["orange"], 0.35 * g, ACCENT["orange"], 2.0, g)]


def split_on(P):
    return P["split"] > 1e-3


def layer_geom(i):
    def geom(P):
        s = P["split"]
        grow = 1 + 0.8 * s
        t0 = STACK_H / STACK_LAYERS
        t = t0 + (0.07 - t0) * s
        z = SUB_Z + lvl3(P)[2] + 1.1 * s + i * (t + 0.32 * s)
        return (SPLIT_STACK[0], SPLIT_STACK[1], z), (0.6 * grow, 0.8 * grow, t)

    return geom


def layer_deco(i):
    def deco(b, F, P):
        s = P["split"] * b.a
        if s < 0.02:
            return []
        out = [F.rect(0.1, 0.1, 0.9, 0.9, BLACK, 0.0, "#9fb3d6" if i else "#9fd4a0", 0.6, 0.45 * s)]
        if i < STACK_LAYERS - 1:
            gap = 0.32 * P["split"]
            for u in (0.3, 0.5, 0.7):
                a = b.top_uv(u, 0.5)
                out.append(polyline(F.cam.proj(np.array([a, a + [0, 0, gap]])), "#e2b866", 1.4, 0.8 * s))
                phase = (P["t"] * 0.8 + u + i * 0.2) % 1.0
                out += glow_dot(F.cam.p1(a + np.array([0, 0, gap * phase])), "#ffd27a", 0.018, 0.9 * s)
        return out

    return deco


class Video(Base):
    DEFAULTS = dict(
        a_board=1.0,
        a_pkg=1.0,
        a_mem=1.0,
        a_stack=1.0,
        a_io=1.0,
        explode=0.0,
        split=0.0,
        mem_glow=0.0,
        link=0.0,
    )

    def world(self):
        boxes = [Box("board", (0, 0, 0), (9.0, 6.4, BOARD_Z), "#141416", "board", gloss=0.06, edge="#3a3a40", edge_op=0.6)]
        for i in range(6):
            boxes.append(Box(f"conn{i}", (-3.0 + i * 1.2, -2.85, BOARD_Z), (0.6, 0.4, 0.25), "#2b2b30", "board", parent="board", gloss=0.15))
        boxes.append(
            Box("substrate", (0, 0.3, BOARD_Z), (4.6, 3.4, 0.08), "#202024", "pkg", parent="board", offset=lvl2, gloss=0.08, edge="#5c5c66", edge_op=0.7)
        )
        boxes.append(Box("die", (0, 0.3, SUB_Z), (2.0, 1.7, 0.08), "#1b1d24", "pkg", parent="substrate", offset=lvl3, deco=die_deco, gloss=0.12, edge="#56607a"))
        for x in (-1.65, 1.65):
            for y in (-0.25, 0.85):
                alpha = (lambda P: 0.0 if split_on(P) else 1.0) if (x, y) == SPLIT_STACK else None
                boxes.append(
                    Box(f"mem{x:+.2f}{y:+.2f}", (x, y, SUB_Z), (0.6, 0.8, STACK_H), "#a7884f", "mem", parent="substrate", offset=lvl3, alpha=alpha, deco=mem_deco, metal=True, gloss=0.14, edge="#e8cf96")
                )
        for i in range(STACK_LAYERS):
            boxes.append(
                Box(
                    f"layer{i}",
                    (0, 0, 0),
                    (1, 1, 1),
                    "#203a2c" if i == 0 else "#28303e",
                    "stack",
                    parent="substrate",
                    geom=layer_geom(i),
                    alpha=lambda P: 1.0 if split_on(P) else 0.0,
                    deco=layer_deco(i),
                    gloss=0.18,
                    edge="#8fd0a0" if i == 0 else "#9fb3d6",
                    edge_op=0.6,
                )
            )
        boxes.append(Box("io", (2.9, -1.9, BOARD_Z), (1.0, 1.0, 0.06), "#2a2a2e", "io", parent="board", offset=lvl2, gloss=0.16, edge="#6a6a72"))
        return boxes

    def overlays(self, B, cam, P):
        k = P["link"]
        if k < 0.01:
            return []
        path = cam.proj(bezier3(B["io"].top_center, B["die"].top_uv(0.85, 0.1), 1.3, 26))
        out = glow_line(path, ACCENT["blue"], 2.6, k)
        for j in range(3):
            out += glow_dot(path_point(path, (P["t"] * 0.45 + j / 3) % 1), "#9fd0ff", 0.03, k)
        return out


POSE_INTRO = dict(theta=-92.0, phi=74.0, zoom=0.5, tx=0.0, ty=0.0, tz=0.0, sx=2.4, sy=0.0)
POSE_HERO = dict(theta=-118.0, phi=38.0, zoom=0.7, tx=0.0, ty=0.0, tz=0.0, sx=2.8, sy=-0.3)
POSE_EXPLODE = dict(theta=-126.0, phi=20.0, zoom=0.8, tx=0.0, ty=0.0, tz=1.2, sx=1.4, sy=-0.2)
POSE_STACK = dict(theta=-122.0, phi=18.0, zoom=1.9, tx=SPLIT_STACK[0], ty=SPLIT_STACK[1], tz=2.1, sx=2.2, sy=0.0)
DIM_FOR_STACK = dict(a_board=0.4, a_pkg=0.3, a_mem=0.3, a_io=0.3)
ALL_ON = dict(a_board=1.0, a_pkg=1.0, a_mem=1.0, a_io=1.0)

# Key frames for preview.py. "t" sets the clock that drives the moving pulses.
PREVIEW_STATES = {
    "hero": dict(**POSE_HERO),
    "explode": dict(explode=1.0, **POSE_EXPLODE),
    "stack": dict(**POSE_STACK, **DIM_FOR_STACK, split=1.0, t=0.4),
    "link": dict(**POSE_HERO, link=1.0, mem_glow=1.0, t=0.3),
}


class S1Title(Video):
    START = dict(reveal=0.0)

    def construct(self):
        eb = eyebrow("EXAMPLE VIDEO", GRAY)
        title = Text("Inside the package", font=FONT, weight=BOLD, font_size=72, color=TXT)
        sub = Text("A compute die, its memory, and the board under them.", font=FONT, font_size=26, color=GRAY)
        g = VGroup(eb, title, sub).arrange(DOWN, buff=0.3)
        self.play(FadeIn(eb, shift=0.15 * UP), run_time=0.8)
        self.play(FadeIn(title, shift=0.25 * UP), run_time=1.1)
        self.play(FadeIn(sub, shift=0.15 * UP), run_time=0.9)
        self.wait(1.5)
        self.play(FadeOut(g, shift=0.2 * UP), run_time=1.0)


class S2Package(Video):
    START = dict(reveal=0.0, **POSE_INTRO)

    def construct(self):
        self.to(reveal=1.0, **POSE_HERO, run_time=5.0, rate=rate_functions.ease_out_cubic)
        tb = text_block(
            "THE PACKAGE",
            "One package.\nMany parts.",
            "A compute die and four memory stacks\nshare one substrate on the board.",
            ACCENT["blue"],
        )
        self.play(FadeIn(tb, shift=0.2 * UP), run_time=1.0)
        B = self.B
        c1 = self.callout(B["die"].top_uv(0.5, 0.9), "Compute die", to=(0.0, 1.4), up=True)
        c2 = self.callout(B["mem-1.65-0.25"].top_uv(0.3, 0.2), "Memory stacks", to=(-0.9, -1.2))
        c3 = self.callout(B["io"].top_uv(0.5, 0.2), "I/O chip", to=(-0.8, -1.0))
        self.show(c1, c2, c3, run_time=1.2)
        self.wait(2.5)
        self.fade(c1, c2, c3, tb)

        self.to(explode=1.0, **POSE_EXPLODE, run_time=4.0)
        tb = text_block("EXPLODED VIEW", "Built in layers.", None, ACCENT["blue"])
        l1 = self.callout(B["die"].top_uv(0.0, 0.3), "Silicon", "Compute die and memory", to=(-1.3, 0.3))
        l2 = self.callout(B["substrate"].top_uv(0.0, 0.3), "Substrate", to=(-1.1, -0.1))
        l3 = self.callout(B["board"].top_uv(0.0, 0.4), "Board", to=(-0.8, -0.45))
        self.play(FadeIn(tb, shift=0.2 * UP), run_time=0.8)
        self.show(l1, l2, l3, run_time=1.2)
        self.wait(2.5)
        self.fade(l1, l2, l3, tb)
        self.to(explode=0.0, **POSE_HERO, run_time=4.0)


class S3Memory(Video):
    START = dict(**POSE_HERO)

    def construct(self):
        self.to(**POSE_STACK, **DIM_FOR_STACK, split=1.0, run_time=5.0)
        tb = text_block(
            "MEMORY",
            "Stacked\nfor bandwidth.",
            "Each stack is a base die under\nfour DRAM dies, wired vertically.",
            ACCENT["orange"],
            with_scrim=True,
        )
        self.play(FadeIn(tb, shift=0.2 * UP), run_time=1.0)
        B = self.B
        c1 = self.callout(B["layer4"].top_uv(1.0, 0.2), "DRAM dies", to=(0.9, 0.4))
        c2 = self.callout(B["layer0"].top_uv(1.0, 0.2), "Base die", to=(0.9, -0.4))
        self.show(c1, c2, run_time=1.0)
        self.wait(2.5)
        self.fade(c1, c2, tb)
        self.to(split=0.0, **POSE_HERO, **ALL_ON, run_time=4.0)

        tb = text_block(
            "I/O",
            "Data in,\ndata out.",
            "Pulses ride a glowing path from\nthe I/O chip to the compute die.",
            ACCENT["blue"],
        )
        self.play(FadeIn(tb, shift=0.2 * UP), run_time=1.0)
        self.to(link=1.0, mem_glow=1.0, run_time=1.5)
        self.wait(3.0)
        self.fade(tb)
        self.to(link=0.0, mem_glow=0.0, reveal=0.0, run_time=2.0)

        head = headline("Closer is faster.", size=38).move_to(np.array([-6.3, 3.2, 0]), aligned_edge=UL)
        rows = hbars(
            [
                ("Memory", "die ↔ stacks", 8.0, "8 units", ACCENT["orange"]),
                ("I/O", "chip ↔ die", 2.0, "2 units", ACCENT["blue"]),
                ("Board", "for comparison", 0.5, "0.5 units", ACCENT["neutral"]),
            ],
            max_value=8.0,
        )
        foot = Text("Illustrative values.", font=FONT, font_size=14, color="#6e6e73").move_to(np.array([0, -3.4, 0]))
        self.play(FadeIn(head, shift=0.2 * UP), run_time=1.0)
        self.show_bars(rows)
        self.play(FadeIn(foot), run_time=0.5)
        self.wait(2.5)
        self.fade(head, foot, *rows)
