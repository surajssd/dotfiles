#!/usr/bin/env bash
# Make review images from the per-scene MP4s that render.sh printed.
#
# Usage: review.sh VIDEO.py SCENE_DIR [OUT_DIR]
#   STEP  seconds between frames on a contact sheet (default 2.5)
#
# Writes OUT_DIR/<Scene>.png, one contact sheet per scene with 4 frames per
# row, and OUT_DIR/seams.png, which puts the last frame of each scene beside
# the first frame of the next one. The two frames of a seam must match.
set -euo pipefail

if [ $# -lt 2 ]; then
    echo "usage: $0 VIDEO.py SCENE_DIR [OUT_DIR]" >&2
    exit 2
fi

module=$1
scene_dir=$2
out=${3:-$(mktemp -d)}
STEP="${STEP:-2.5}"
mkdir -p "$out"

scenes=()
while IFS= read -r name; do
    scenes+=("$name")
done < <(sed -nE 's/^class (S[0-9]+[A-Za-z0-9_]*)\(.*/\1/p' "$module")

for scene in "${scenes[@]}"; do
    clip="$scene_dir/$scene.mp4"
    duration=$(ffprobe -v error -show_entries format=duration -of csv=p=0 "$clip")
    rows=$(awk -v d="$duration" -v s="$STEP" 'BEGIN { n = int(d / s) + 1; print int((n + 3) / 4) }')
    ffmpeg -v error -y -i "$clip" \
        -vf "fps=1/$STEP,scale=640:-1,tile=4x$rows:padding=4:color=white" \
        -frames:v 1 "$out/$scene.png"
    echo "$out/$scene.png"
done

if [ ${#scenes[@]} -gt 1 ]; then
    frames=$(mktemp -d)
    n=0
    for i in $(seq 0 $((${#scenes[@]} - 2))); do
        ffmpeg -v error -y -sseof -0.5 -i "$scene_dir/${scenes[$i]}.mp4" \
            -vf scale=640:-1 -update 1 "$frames/$(printf %03d "$n").png"
        ffmpeg -v error -y -i "$scene_dir/${scenes[$((i + 1))]}.mp4" \
            -vf scale=640:-1 -frames:v 1 "$frames/$(printf %03d $((n + 1))).png"
        n=$((n + 2))
    done
    ffmpeg -v error -y -framerate 1 -i "$frames/%03d.png" \
        -vf "tile=2x$((n / 2)):padding=4:color=white" -frames:v 1 "$out/seams.png"
    rm -rf "$frames"
    echo "$out/seams.png"
fi
