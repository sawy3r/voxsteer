#!/usr/bin/env bash
# Renders the VoxSteer artwork into an .icns file.

set -euo pipefail

if [[ $# -ne 2 ]]; then
  printf 'usage: %s <work-dir> <output.icns>\n' "$0" >&2
  exit 2
fi

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
ICON_SOURCE="$PROJECT_ROOT/cmd/voxsteer/assets/voxsteer.png"
ICONSET="$1/VoxSteer.iconset"

mkdir -p "$ICONSET"
for spec in \
  "16 icon_16x16.png" \
  "32 icon_16x16@2x.png" \
  "32 icon_32x32.png" \
  "64 icon_32x32@2x.png" \
  "128 icon_128x128.png" \
  "256 icon_128x128@2x.png" \
  "256 icon_256x256.png" \
  "512 icon_256x256@2x.png" \
  "512 icon_512x512.png" \
  "1024 icon_512x512@2x.png"; do
  read -r size name <<< "$spec"
  sips -z "$size" "$size" "$ICON_SOURCE" --out "$ICONSET/$name" >/dev/null
done
iconutil -c icns "$ICONSET" -o "$2"
