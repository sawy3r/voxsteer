//go:build desktop && !darwin

package desktop

// Only macOS gates microphone and input access behind per-application consent
// that VoxSteer has to request itself.

func RequestPermissions() {}

func InputAccessGranted() bool { return true }

func MissingPermission() Permission { return Permission{} }

func WatchPermissions(func(Permission)) {}

func microphoneReady() error { return nil }
