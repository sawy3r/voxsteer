#!/usr/bin/env bash
# Builds the native Universal 2 VoxSteer.app.

set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION="${VERSION:-0.1.0}"
BUILD_NUMBER="${BUILD_NUMBER:-1}"
OUTPUT_DIR="${OUTPUT_DIR:-$PROJECT_ROOT/dist/macos}"
BUILD_DIR="$PROJECT_ROOT/.build/macos"
APP="$OUTPUT_DIR/VoxSteer.app"
IDENTITY="${MACOS_SIGN_IDENTITY:--}"
ENTITLEMENTS="$PROJECT_ROOT/packaging/macos/VoxSteer.entitlements"

if [[ "$(uname -s)" != "Darwin" ]]; then
  printf 'build-macos.sh must run on macOS\n' >&2
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
mkdir -p "$BUILD_DIR" "$APP/Contents/MacOS" "$APP/Contents/Resources"

cp packaging/macos/Info.plist "$APP/Contents/Info.plist"
plutil -replace CFBundleShortVersionString -string "$VERSION" "$APP/Contents/Info.plist"
plutil -replace CFBundleVersion -string "$BUILD_NUMBER" "$APP/Contents/Info.plist"
plutil -lint "$APP/Contents/Info.plist"

# Without a deployment target clang stamps the binary with the build machine's
# macOS version, which would not match LSMinimumSystemVersion.
MACOSX_DEPLOYMENT_TARGET="$(plutil -extract LSMinimumSystemVersion raw "$APP/Contents/Info.plist")"
export MACOSX_DEPLOYMENT_TARGET

for spec in "arm64 arm64" "amd64 x86_64"; do
  read -r goarch clangarch <<< "$spec"
  CGO_ENABLED=1 GOOS=darwin GOARCH="$goarch" CC="clang -arch $clangarch" go build \
    -buildvcs=false \
    -tags desktop \
    -trimpath \
    -ldflags="-s -w" \
    -o "$BUILD_DIR/voxsteer-$goarch" \
    ./cmd/voxsteer
done
lipo -create \
  "$BUILD_DIR/voxsteer-arm64" \
  "$BUILD_DIR/voxsteer-amd64" \
  -output "$APP/Contents/MacOS/voxsteer"

bash "$PROJECT_ROOT/scripts/make-macos-icon.sh" "$BUILD_DIR" "$APP/Contents/Resources/VoxSteer.icns"

# Sign ad-hoc builds with the hardened runtime too, so a missing entitlement
# fails here rather than only in a Developer ID release.
SIGN_ARGS=(--force --options runtime --entitlements "$ENTITLEMENTS" --sign "$IDENTITY")
if [[ "$IDENTITY" != "-" ]]; then
  SIGN_ARGS+=(--timestamp)
fi
codesign "${SIGN_ARGS[@]}" "$APP"
codesign --verify --strict --verbose=2 "$APP"

printf 'Built %s\n' "$APP"
