---
name: manim-video
description: Make or edit an Apple-style explainer video with Manim (3Blue1Brown's animation library), with on-screen text and no audio. It draws exploded 3D views of hardware such as boards, chips, and memory stacks, labeled callouts, glowing data links, and bar charts. Use when the user asks for an explainer video, an animation, or a Manim scene about a system or hardware topic, or asks to change or re-render a video built with this skill. Do not use for a static figure or chart.
---

# Manim explainer videos

This skill makes short explainer videos in the style of Apple's hardware films: a black background, slow camera moves around a 3D model, parts that lift apart in exploded views, and short headlines with thin callout lines. A video is a Python module built on `engine.py`, rendered scene by scene and joined into one MP4.

## Files

| File | What it is |
|---|---|
| `assets/engine.py` | The engine: camera, boxes, depth sort, shadows, glow effects, text styles, callouts, bar charts, and the `Base` scene. Copy it into every video folder. |
| `assets/template.py` | A working three-scene video (a board, a die, memory stacks, an I/O chip) that uses every technique below. Start every new video from it. |
| `scripts/preview.py` | Renders still frames of the named states in `PREVIEW_STATES`, plus `sheet.png`, which tiles them. Each still takes about 6 seconds. |
| `scripts/render.sh` | Renders every scene in parallel and joins them. On 18 cores, a 480p15 draft of a 3.5-minute video takes about 35 seconds and the 1080p60 final takes about 4 minutes. |
| `scripts/review.sh` | Makes one contact sheet per scene and `seams.png`, which shows the last frame of each scene beside the first frame of the next one. |

Resolve the skill directory before you use these files. `~/.claude/skills/manim-video` can be a symlink, and BSD `readlink` has no `-f`:

```bash
SKILL_LINK=~/.claude/skills/manim-video/SKILL.md
SKILL_TARGET="$(readlink "$SKILL_LINK" 2>/dev/null || echo "$SKILL_LINK")"
SKILL_DIR="$(cd "$(dirname "$SKILL_TARGET")" && pwd -P)"
```

The scripts pin Python 3.12 and manim 0.21.0 through `uv`, and they need `ffmpeg` and `ffprobe` on the path. Nothing else needs to be installed. Python 3.14 is untested.

## Workflow

1. Collect the facts from the source material that the user names. For a PDF, extract the text and find the section in it:

   ```bash
   uv run --with pypdf python -c 'import pypdf, sys
   r = pypdf.PdfReader(sys.argv[1])
   for i, p in enumerate(r.pages): print(f"=== PAGE {i+1} ===\n" + (p.extract_text() or ""))' SOURCE.pdf > source.txt
   ```

   Extract the figures as well (`page.images` in pypdf) and look at them. The physical layout of the 3D model comes from the figures.
2. List every number that the video will show, with where it came from. The source material wins. When you add a fact from elsewhere, check it and tell the user which facts those are. When you derive a number that differs from a rounded figure in the source, show the arithmetic on screen, for example 480 + 2 × 192 = 864 GB where the source says "~900 GB".
3. Write a storyboard. Give each scene one row with its camera pose, the trackers it animates, its text, and the state it ends in. Keep the whole video between 1 and 4 minutes.
4. Create the video folder where the user wants it. If the user did not say, ask. Copy the engine and the template into it:

   ```bash
   mkdir -p VIDEO_DIR
   cp "$SKILL_DIR/assets/engine.py" VIDEO_DIR/
   cp "$SKILL_DIR/assets/template.py" VIDEO_DIR/NAME.py
   ```

5. Replace the world, the overlays, the poses, the text, and the scenes in `NAME.py` (see "Module layout"). Add key frames to `PREVIEW_STATES`. Run `uv run "$SKILL_DIR/scripts/preview.py" VIDEO_DIR/NAME.py --out DIR` and read `sheet.png`. Repeat until every pose is framed: the model sits in the right half, nothing important is cropped, and the left column is free for text.
6. Render a draft with `QUALITY=l MEDIA_DIR=DIR "$SKILL_DIR/scripts/render.sh" VIDEO_DIR/NAME.py`. Run `"$SKILL_DIR/scripts/review.sh" VIDEO_DIR/NAME.py SCENE_DIR OUT_DIR` on the scene directory that `render.sh` prints. Read every contact sheet and `seams.png`. Fix text that overlaps the model, callouts that are cropped or overlap each other, and seams whose two frames differ. Repeat this step until the sheets are clean.
7. Render the final video with `"$SKILL_DIR/scripts/render.sh" VIDEO_DIR/NAME.py`. It writes `NAME.mp4` beside the module. Run `review.sh` again on the 1080p60 scene directory. Run `ffprobe` on the output and confirm 1920x1080, 60 fps, and the expected duration.

Put drafts, stills, and review images in the session scratchpad. When you are done, the video folder holds `engine.py`, `NAME.py`, and `NAME.mp4`. `render.sh` writes `NAME.mp4` beside the module only at `QUALITY=h`, so a draft cannot replace the final video.

## Module layout

A video module starts with `from manim import *` and `from engine import *`, and it defines these parts:

- `class Video(Base)` with three members:
  - `DEFAULTS`: every tracker that the scenes animate, with its starting value. Include one `a_<group>` opacity tracker for each box group. The engine adds the camera trackers and `reveal`.
  - `world(self)`: returns the list of `Box` objects. Box names must be unique.
  - `overlays(self, B, cam, P)`: returns links, pulses, and other marks that are drawn above every box. `B` maps box names to boxes.
- Pose dicts such as `POSE_HERO`, which hold camera tracker values, and dimming dicts such as `DIM_FOR_STACK`, which hold `a_<group>` values.
- `PREVIEW_STATES`: state names mapped to tracker values. A `"t"` key sets the clock.
- Scene classes named `S<number><Name>`, such as `S1Title`, that subclass `Video` and set `START`. `render.sh` and `review.sh` find scenes by that name pattern and keep the file order. `Base` raises an error when `START` sets a key that `DEFAULTS` lacks.

## Engine reference

- **World axes.** x points right, y points away from a camera at `theta=-90`, and z points up. `Box(name, c, size, color, group)` takes `c`, the center of its bottom face, and `size`, the width, depth, and height.
- **Box options.** `parent` names the box that receives this box's shadow. `offset(P)` returns a 3D shift. `alpha(P)` returns an opacity factor. `geom(P)` returns `(c, size)` and overrides both. `deco(b, F, P)` returns marks on the top face. `metal=True` and `gloss` set the shine, and `edge` and `edge_op` set the bevel line.
- **Top-face helpers.** In `deco`, `F.rect(u0, v0, u1, v1, fill, opacity, stroke, width, stroke_opacity)` draws a rectangle in face coordinates from 0 to 1. `F.screen_width()` gives the face's width on screen, so fine detail can appear only when the face is large. `b.top_uv(u, v)` and `b.top_center` return world points on the face.
- **Camera trackers.** `theta` is the azimuth, and `-90` looks from the front. `phi` is the elevation in degrees. `zoom` scales the picture. `tx`, `ty`, and `tz` set the point that the camera looks at. `sx` and `sy` shift that point on screen. `dist` sets the strength of the perspective.
- **Animation.** `self.to(key=value, ..., run_time=3)` animates any set of trackers with one ease. Extra Manim animations can go in the same call as positional arguments. `P["t"]` is the clock in seconds, for looping pulses.
- **Effects.** `glow_line`, `glow_dot`, `bezier3`, `path_point`, `path_upto`, `shade`, and `mix`.
- **Text.** `text_block(eyebrow, headline, body, color, with_scrim=False)`, `self.callout(world_point, title, sub, to=(dx, dy), up=False)` with `self.show(...)`, `hbars(rows, max_value)` with `self.show_bars(rows)`, and `self.fade(...)`. `ACCENT` holds six accent colors.

## Techniques

The template shows each of these techniques.

- **Reveal.** Start a scene with `reveal=0` and a high camera pose, then animate `reveal` to 1 while the camera moves to the hero pose.
- **Exploded view.** Give each layer an `offset` that lifts it by a multiple of the `explode` tracker. Lower `phi` to between 15 and 20 degrees so the gaps show.
- **Part that splits into layers.** Hide the part with `alpha` while a split tracker is above 0, and show one layer box per slice with `geom`. Grow the layers' footprint only after they have lifted clear of their neighbors, because the depth sort needs boxes that do not intersect.
- **Focus.** Dim the other groups through their `a_<group>` trackers, and pass `with_scrim=True` to `text_block` when boxes sit behind the text.
- **Data links.** Draw a `bezier3` path in `overlays` with `glow_line`, and move `glow_dot` pulses along it with a phase from `P["t"]`.
- **Charts.** Fade the model out with `reveal=0`, then build bars with `hbars` on a linear scale. Label illustrative values as illustrative.

## Scene contract

- Each scene must end in exactly the state the next scene starts in, because `render.sh` joins the scenes with a plain concat. Build `START` from the same pose and dimming dicts that the previous scene animated to, and reset every temporary tracker before the scene ends.
- A boundary on a black frame, with `reveal=0` on both sides, also works.
- Do not leave looping pulses on screen at a boundary. The clock starts at 0 in each scene, so the pulses jump.
- Do not use `hash()` for animation phases. Its value changes between processes, and every scene renders in its own process.
- Show callouts only while the camera is still, because `callout()` places its label from the current camera. Fade callouts out before the next camera move.

## Problems the engine already solves

- **Stale copy of the picture.** At the start of each `play()`, stock Manim makes a flat list of the moving mobjects and their children. The polygons that `Stage` had at that moment then stay on screen as a frozen second copy. `Base.get_moving_and_static_mobjects` returns only the top-level mobjects, which fixes this. Keep the override.
- **ThreeDScene.** Manim's Cairo 3D camera sorts faces by their centers, so it draws a large board over the small chips that sit on it. This is why the engine has its own projection. Keep `phi` between 0 and 89 degrees, because the sort assumes that the camera is above the boxes.
- **Fonts.** Pango cannot see SF Pro, so the engine uses Helvetica Neue. To list the fonts that Pango can use, run `import manimpango; print(manimpango.list_fonts())` in the manim environment.
- **Line spacing.** A `Text` that contains a newline puts very large gaps between its lines. Use `headline()` and `body()`, which make one `Text` per line.
- **Letter spacing.** The eyebrows use `MarkupText` with `letter_spacing="1800"`.
- **Cropped callouts.** The model fills the right half of the frame, so a label that points right gets cropped at the frame edge. Point callouts left or down, or pass `up=True` to center the label above its anchor.
- **Cost per frame.** The picture is rebuilt on every frame, so do not create a `Text` inside `deco` or `overlays`. Create text once, outside the frame loop.

## Style

- Use a black background, a white headline (`#f5f5f7`), gray body text (`#86868b`), and Helvetica Neue.
- Put text in a left column with its top-left corner at `(-6.55, 2.95)`: a colored eyebrow, a bold headline of one or two lines, and two to four lines of body text. Put the model to the right, with the screen shift `sx` between about 2.3 and 3.4.
- Give each kind of link or memory one accent color, and use it in every scene.
- Move the camera slowly, over 3 to 5 seconds, with the default ease. Hold each text block on screen for 3 to 4 seconds after it appears.
