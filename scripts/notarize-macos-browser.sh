#!/usr/bin/env bash

set -euo pipefail

if [[ $# -ne 3 ]]; then
  printf 'usage: %s <app> <zip> <dmg>\n' "$0" >&2
  exit 2
fi

APP="$1"
ZIP="$2"
DMG="$3"
IDENTITY="${MACOS_SIGN_IDENTITY:?MACOS_SIGN_IDENTITY is required}"

if [[ "$(uname -s)" != "Darwin" ]]; then
  printf 'notarize-macos-browser.sh must run on macOS\n' >&2
  exit 1
fi
if [[ ! -d "$APP" || ! -f "$ZIP" || ! -f "$DMG" ]]; then
  printf 'app, ZIP, and DMG must already exist\n' >&2
  exit 1
fi

NOTARY_ARGS=()
if [[ -n "${APPLE_NOTARY_KEYCHAIN_PROFILE:-}" ]]; then
  NOTARY_ARGS+=(--keychain-profile "$APPLE_NOTARY_KEYCHAIN_PROFILE")
else
  NOTARY_ARGS+=(
    --key "${APPLE_API_KEY_FILE:?APPLE_API_KEY_FILE is required}"
    --key-id "${APPLE_API_KEY_ID:?APPLE_API_KEY_ID is required}"
    --issuer "${APPLE_API_ISSUER_ID:?APPLE_API_ISSUER_ID is required}"
  )
fi

codesign --verify --deep --strict --verbose=2 "$APP"
xcrun notarytool submit "$ZIP" "${NOTARY_ARGS[@]}" --wait
xcrun stapler staple "$APP"
xcrun stapler validate "$APP"

rm -f "$ZIP"
ditto -c -k --sequesterRsrc --keepParent "$APP" "$ZIP"

STAGING="$(mktemp -d "${TMPDIR:-/tmp}/voxsteer-browser-dmg.XXXXXX")"
trap 'rm -rf "$STAGING"' EXIT
ditto "$APP" "$STAGING/$(basename "$APP")"
ln -s /Applications "$STAGING/Applications"
rm -f "$DMG"
hdiutil create -quiet -fs HFS+ -format UDZO -volname "VoxSteer Browser" -srcfolder "$STAGING" "$DMG"
codesign --force --timestamp --sign "$IDENTITY" "$DMG"
xcrun notarytool submit "$DMG" "${NOTARY_ARGS[@]}" --wait
xcrun stapler staple "$DMG"
xcrun stapler validate "$DMG"
codesign --verify --verbose=2 "$DMG"
hdiutil verify "$DMG"
spctl --assess --type execute --verbose=2 "$APP"
spctl --assess --type open --context context:primary-signature --verbose=2 "$DMG"

printf 'Notarized and stapled %s\n' "$APP"
printf 'Notarized and stapled %s\n' "$DMG"
