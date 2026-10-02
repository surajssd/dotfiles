#!/usr/bin/env bash
# Render every scene of a Manim video module in parallel, then join them.
#
# Usage: render.sh VIDEO.py [OUTPUT.mp4]
#   QUALITY   h (default, 1080p60), m (720p30), or l (480p15)
#   MEDIA_DIR where manim writes scenes and logs (default: a new temp dir)
#
# Scenes are the classes named S<number><Name>, such as S1Intro, in file order. The
# join is a plain concat, so each scene must end in the state the next one
# starts in. The default output is VIDEO.mp4 beside the module at QUALITY=h and
# MEDIA_DIR/VIDEO-QUALITY.mp4 otherwise, so a draft never replaces the final.
set -euo pipefail

if [ $# -lt 1 ]; then
    echo "usage: $0 VIDEO.py [OUTPUT.mp4]" >&2
    exit 2
fi

video_dir=$(cd "$(dirname "$1")" && pwd)
module=$(basename "$1")
stem=${module%.py}
QUALITY="${QUALITY:-h}"
MEDIA_DIR="${MEDIA_DIR:-$(mktemp -d)}"
mkdir -p "$MEDIA_DIR"
MEDIA_DIR=$(cd "$MEDIA_DIR" && pwd)

case "$QUALITY" in
h) res=1080p60 ;;
m) res=720p30 ;;
l) res=480p15 ;;
*)
    echo "QUALITY must be h, m, or l" >&2
    exit 2
    ;;
esac

if [ $# -ge 2 ]; then
    out=$2
elif [ "$QUALITY" = h ]; then
    out="$video_dir/$stem.mp4"
else
    out="$MEDIA_DIR/$stem-$QUALITY.mp4"
fi

scenes=()
while IFS= read -r name; do
    scenes+=("$name")
done < <(sed -nE 's/^class (S[0-9]+[A-Za-z0-9_]*)\(.*/\1/p' "$video_dir/$module")
if [ ${#scenes[@]} -eq 0 ]; then
    echo "no scene classes named S<number><Name> found in $module" >&2
    exit 1
fi

cd "$video_dir"
export PYTHONDONTWRITEBYTECODE=1
pids=()
for scene in "${scenes[@]}"; do
    uv run --quiet --python 3.12 --with "manim==0.21.0" \
        manim "-q$QUALITY" --media_dir "$MEDIA_DIR" "$module" "$scene" >"$MEDIA_DIR/$scene.log" 2>&1 &
    pids+=("$!")
done
failed=0
for i in "${!pids[@]}"; do
    if ! wait "${pids[$i]}"; then
        echo "render failed: ${scenes[$i]} (see $MEDIA_DIR/${scenes[$i]}.log)" >&2
        failed=1
    fi
done
if [ "$failed" -ne 0 ]; then
    exit 1
fi

scene_dir="$MEDIA_DIR/videos/$stem/$res"
list="$MEDIA_DIR/concat.txt"
: >"$list"
for scene in "${scenes[@]}"; do
    echo "file '$scene_dir/$scene.mp4'" >>"$list"
done
ffmpeg -v error -y -f concat -safe 0 -i "$list" -c copy -movflags +faststart "$out"
echo "scenes: $scene_dir"
echo "video:  $out"
