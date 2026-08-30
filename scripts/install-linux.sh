#!/bin/sh
# Installs the built binary, icon, and an autostart entry for the current user.
set -eu

# Works both from a repository checkout (bin/voxsteer) and from an unpacked
# release tarball (voxsteer beside this script).
script_dir="$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
binary=""
icon=""
for candidate in "$script_dir/../bin/voxsteer" "$script_dir/voxsteer"; do
    if [ -f "$candidate" ]; then
        binary="$candidate"
        break
    fi
done
for candidate in "$script_dir/../cmd/voxsteer/assets/voxsteer.png" "$script_dir/voxsteer.png"; do
    if [ -f "$candidate" ]; then
        icon="$candidate"
        break
    fi
done
if [ -z "$binary" ]; then
    echo "voxsteer binary not found; run 'make build-linux' first" >&2
    exit 1
fi

bin_dir="${XDG_BIN_HOME:-$HOME/.local/bin}"
data_dir="${XDG_DATA_HOME:-$HOME/.local/share}"
autostart_dir="${XDG_CONFIG_HOME:-$HOME/.config}/autostart"
icon_dir="$data_dir/icons/hicolor/256x256/apps"

mkdir -p "$bin_dir" "$autostart_dir" "$icon_dir" "$data_dir/applications"
install -m 0755 "$binary" "$bin_dir/voxsteer"
if [ -n "$icon" ]; then
    install -m 0644 "$icon" "$icon_dir/voxsteer.png"
fi

desktop_entry="[Desktop Entry]
Type=Application
Name=VoxSteer
Comment=Voice dictation into the focused application
Exec=$bin_dir/voxsteer
Icon=voxsteer
Terminal=false
Categories=Utility;Accessibility;
X-GNOME-Autostart-enabled=true"

printf '%s\n' "$desktop_entry" > "$data_dir/applications/voxsteer.desktop"
printf '%s\n' "$desktop_entry" > "$autostart_dir/voxsteer.desktop"

echo "Installed $bin_dir/voxsteer and an autostart entry."
echo
echo "Activation options (pick one):"
echo "  1. Desktop shortcut (recommended, works everywhere): bind a system"
echo "     keyboard shortcut to: $bin_dir/voxsteer toggle"
echo "  2. Built-in shortcut listener: add yourself to the input group and"
echo "     log out and back in: sudo usermod -aG input \$USER"
echo
echo "Typing tool:"
echo "  Wayland: sudo apt install wtype     (or ydotool)"
echo "  X11:     sudo apt install xdotool"
