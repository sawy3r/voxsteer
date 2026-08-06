#!/usr/bin/env bash

set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION="${VERSION:-0.1.0}"
BUILD_NUMBER="${BUILD_NUMBER:-1}"
OUTPUT_DIR="${OUTPUT_DIR:-$PROJECT_ROOT/dist/macos-browser}"
BUILD_DIR="$PROJECT_ROOT/.build/macos-browser"
APP_NAME="VoxSteer Browser.app"
APP="$OUTPUT_DIR/$APP_NAME"
IDENTITY="${MACOS_SIGN_IDENTITY:--}"
BUNDLE_ID="com.sawy3r.voxsteer.browser"

if [[ "$(uname -s)" != "Darwin" ]]; then
  printf 'build-macos-browser.sh must run on macOS\n' >&2
  exit 1
fi
if [[ ! "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  printf 'VERSION must be a dotted release version, got %s\n' "$VERSION" >&2
  exit 1
fi
if [[ ! "$BUILD_NUMBER" =~ ^[0-9]+$ ]]; then
  printf 'BUILD_NUMBER must be numeric, got %s\n' "$BUILD_NUMBER" >&2
  exit 1
fi

cd "$PROJECT_ROOT"
rm -rf "$BUILD_DIR" "$APP"
mkdir -p "$BUILD_DIR" "$APP/Contents/MacOS" "$APP/Contents/Resources" "$OUTPUT_DIR"

for arch in arm64 amd64; do
  CGO_ENABLED=0 GOOS=darwin GOARCH="$arch" go build \
    -buildvcs=false \
    -trimpath \
    -ldflags="-s -w -X main.version=$VERSION" \
    -o "$BUILD_DIR/voxsteer-browser-$arch" \
    ./cmd/voxsteer-browser
done
lipo -create \
  "$BUILD_DIR/voxsteer-browser-arm64" \
  "$BUILD_DIR/voxsteer-browser-amd64" \
  -output "$APP/Contents/MacOS/voxsteer-browser"

cp "$PROJECT_ROOT/packaging/macos-browser/Info.plist" "$APP/Contents/Info.plist"
plutil -replace CFBundleShortVersionString -string "$VERSION" "$APP/Contents/Info.plist"
plutil -replace CFBundleVersion -string "$BUILD_NUMBER" "$APP/Contents/Info.plist"

ICONSET="$BUILD_DIR/VoxSteer.iconset"
mkdir -p "$ICONSET"
ICON_SOURCE="$PROJECT_ROOT/cmd/voxsteer/assets/voxsteer.png"
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
iconutil -c icns "$ICONSET" -o "$APP/Contents/Resources/VoxSteer.icns"

plutil -lint "$APP/Contents/Info.plist"
if [[ "$(plutil -extract LSMinimumSystemVersion raw "$APP/Contents/Info.plist")" != "13.0" ]]; then
  printf 'browser bundle must target macOS 13.0 or newer\n' >&2
  exit 1
fi
if plutil -extract NSMicrophoneUsageDescription raw "$APP/Contents/Info.plist" >/dev/null 2>&1; then
  printf 'browser bundle must not declare native microphone use\n' >&2
  exit 1
fi

if [[ "$IDENTITY" == "-" ]]; then
  codesign --force --sign - --identifier "$BUNDLE_ID" "$APP"
else
  codesign --force --options runtime --timestamp --sign "$IDENTITY" "$APP"
fi
codesign --verify --strict --verbose=2 "$APP"

ZIP="$OUTPUT_DIR/voxsteer-$VERSION-macos-universal-browser.zip"
DMG="$OUTPUT_DIR/voxsteer-$VERSION-macos-universal-browser.dmg"
rm -f "$ZIP" "$DMG"
ditto -c -k --sequesterRsrc --keepParent "$APP" "$ZIP"

DMG_ROOT="$BUILD_DIR/dmg"
mkdir -p "$DMG_ROOT"
ditto "$APP" "$DMG_ROOT/$APP_NAME"
ln -s /Applications "$DMG_ROOT/Applications"
hdiutil create -quiet -fs HFS+ -format UDZO -volname "VoxSteer Browser" -srcfolder "$DMG_ROOT" "$DMG"
if [[ "$IDENTITY" != "-" ]]; then
  codesign --force --timestamp --sign "$IDENTITY" "$DMG"
fi

printf 'Built %s\n' "$APP"
printf 'Built %s\n' "$ZIP"
printf 'Built %s\n' "$DMG"
