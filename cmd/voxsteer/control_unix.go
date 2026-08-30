//go:build desktop && !windows

package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
)

// The control socket lets a second invocation (voxsteer toggle) drive the
// running instance. On Linux this is the zero-permission activation path:
// bind a system keyboard shortcut to "voxsteer toggle" and it works on any
// compositor, Wayland or X11.

func controlSocketPath() string {
	if runtimeDir := os.Getenv("XDG_RUNTIME_DIR"); runtimeDir != "" {
		return filepath.Join(runtimeDir, "voxsteer.sock")
	}
	return filepath.Join(os.TempDir(), fmt.Sprintf("voxsteer-%d.sock", os.Getuid()))
}

func startControl(onToggle func()) (func(), error) {
	path := controlSocketPath()
	if conn, err := net.Dial("unix", path); err == nil {
		_ = conn.Close()
		return nil, fmt.Errorf("another VoxSteer instance is already running (control socket %s)", path)
	}
	_ = os.Remove(path)

	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("listen on control socket %s: %w", path, err)
	}
	_ = os.Chmod(path, 0o600)

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				line, err := bufio.NewReader(conn).ReadString('\n')
				if err != nil && line == "" {
					return
				}
				if strings.TrimSpace(line) == "toggle" {
					onToggle()
				}
			}()
		}
	}()
	return func() {
		_ = listener.Close()
		_ = os.Remove(path)
	}, nil
}

func sendToggle() error {
	path := controlSocketPath()
	conn, err := net.Dial("unix", path)
	if err != nil {
		return fmt.Errorf("VoxSteer does not appear to be running (%s): %w", path, err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("toggle\n")); err != nil {
		return err
	}
	return nil
}
