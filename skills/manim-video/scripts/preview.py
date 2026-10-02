# /// script
# requires-python = ">=3.12,<3.13"
# dependencies = ["manim==0.21.0"]
# ///
"""Render still frames of named states from a Manim video module.

The module must define PREVIEW_STATES, a dict that maps a state name to tracker
overrides, and a scene class to render them with: `Video` if it exists, else
`Base`. That class must read the class attribute START in setup() and create
self.clock. A "t" key in a state sets the clock.

    uv run preview.py VIDEO.py [STATE ...] [--out DIR] [--res 1280x720]

Prints the path of each still and of sheet.png, which tiles all of them.
"""

from __future__ import annotations

import argparse
import importlib.util
import subprocess
import sys
import tempfile
from pathlib import Path


def load(path: Path):
    sys.dont_write_bytecode = True
    sys.path.insert(0, str(path.parent))
    spec = importlib.util.spec_from_file_location(path.stem, path)
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod


def still(mod, state: dict, name: str, out: Path, width: int, height: int) -> Path:
    from manim import tempconfig

    state = dict(state)
    t = state.pop("t", 0.0)

    class Still(getattr(mod, "Video", None) or mod.Base):
        START = state

        def setup(self):
            super().setup()
            self.clock.set_value(t)

        def construct(self):
            self.wait(0.05)

    opts = {
        "pixel_width": width,
        "pixel_height": height,
        "save_last_frame": True,
        "write_to_movie": False,
        "disable_caching": True,
        "media_dir": str(out / "media"),
        "images_dir": str(out),
        "output_file": name,
        "verbosity": "WARNING",
        "progress_bar": "none",
    }
    with tempconfig(opts):
        Still().render()
    return out / f"{name}.png"


def main() -> None:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("video", type=Path)
    ap.add_argument("states", nargs="*", help="state names; default: all of PREVIEW_STATES")
    ap.add_argument("--out", type=Path, help="output directory; default: a new temp directory")
    ap.add_argument("--res", default="1280x720")
    args = ap.parse_args()

    mod = load(args.video.resolve())
    names = args.states or list(mod.PREVIEW_STATES)
    unknown = [n for n in names if n not in mod.PREVIEW_STATES]
    if unknown:
        sys.exit(f"unknown states {unknown}; known: {list(mod.PREVIEW_STATES)}")
    out = (args.out or Path(tempfile.mkdtemp(prefix="manim-preview-"))).resolve()
    out.mkdir(parents=True, exist_ok=True)
    width, height = (int(v) for v in args.res.lower().split("x"))

    stills = []
    for i, name in enumerate(names):
        stills.append(still(mod, mod.PREVIEW_STATES[name], f"{i:02d}-{name}", out, width, height))
        print(stills[-1])

    cols = 2 if len(stills) > 1 else 1
    rows = -(-len(stills) // cols)
    sheet = out / "sheet.png"
    subprocess.run(
        [
            "ffmpeg", "-v", "error", "-y",
            "-framerate", "1", "-pattern_type", "glob", "-i", str(out / "[0-9][0-9]-*.png"),
            "-vf", f"scale=960:-1,tile={cols}x{rows}:padding=6:color=white",
            "-frames:v", "1", str(sheet),
        ],
        check=True,
    )
    print(sheet)


if __name__ == "__main__":
    main()
