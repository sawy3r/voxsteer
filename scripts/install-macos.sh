#!/usr/bin/env bash
# Installs VoxSteer.app, starts it at login, and launches it.

set -euo pipefail

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SOURCE="${SOURCE:-$PROJECT_ROOT/dist/macos/VoxSteer.app}"
INSTALL_DIR="${INSTALL_DIR:-/Applications}"
DESTINATION="$INSTALL_DIR/VoxSteer.app"
BUNDLE_ID="com.sawy3r.voxsteer"
LAUNCH_AGENT="$HOME/Library/LaunchAgents/$BUNDLE_ID.plist"
LSREGISTER="/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"

if [[ ! -d "$SOURCE" ]]; then
  printf "%s not found; run 'make build-macos' first\n" "$SOURCE" >&2
  exit 1
fi

# Both helpers read codesign's whole output. Stopping early, as grep -q or head
# would, kills codesign with SIGPIPE, and pipefail turns that into a failure.
cdhash() {
  codesign -dvvv "$1" 2>&1 | sed -n 's/^CDHash=//p'
}

adhoc_signed() {
  codesign -dv "$1" 2>&1 | grep '^Signature=adhoc$' >/dev/null
}

previous_hash=""
if [[ -d "$DESTINATION" ]]; then
  previous_hash="$(cdhash "$DESTINATION")"
  pkill -f "$DESTINATION/Contents/MacOS/voxsteer" || true
fi

mkdir -p "$INSTALL_DIR"
rm -rf "$DESTINATION"
ditto "$SOURCE" "$DESTINATION"

# macOS ties an ad-hoc application's Accessibility grant to this exact build. A
# rebuild leaves the switch on in System Settings while the grant no longer
# applies, so clear it and let VoxSteer prompt again. With nothing installed
# before, the grant may still be from a build run elsewhere, so clear it then
# too. tccutil only recognises applications LaunchServices knows about, so
# register the new copy first.
if [[ "$previous_hash" != "$(cdhash "$DESTINATION")" ]] && adhoc_signed "$DESTINATION"; then
  "$LSREGISTER" -f "$DESTINATION"
  tccutil reset Accessibility "$BUNDLE_ID" >/dev/null ||
    printf 'Could not clear the old Accessibility entry. If VoxSteer does not work, remove it from System Settings and add it again.\n' >&2
fi

mkdir -p "$(dirname "$LAUNCH_AGENT")"
cat > "$LAUNCH_AGENT" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "https://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>$BUNDLE_ID</string>
  <key>ProgramArguments</key>
  <array>
    <string>/usr/bin/open</string>
    <string>-a</string>
    <string>$DESTINATION</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
</dict>
</plist>
EOF
plutil -lint "$LAUNCH_AGENT" >/dev/null

open "$DESTINATION"

printf 'Installed %s and a login item.\n' "$DESTINATION"
printf 'Allow the microphone and Accessibility prompts when they appear.\n'
printf 'To remove VoxSteer, quit it and delete %s and %s\n' "$DESTINATION" "$LAUNCH_AGENT"
