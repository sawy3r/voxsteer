//go:build desktop && !darwin

package settingsui

// Available reports whether this platform has a settings window. Windows sets
// up through its installer, and Linux through the configuration file.
const Available = false

// Open does nothing on this platform.
func Open(Options) {}
