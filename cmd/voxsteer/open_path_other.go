//go:build !windows

package main

import (
	"fmt"
	"os/exec"
	"runtime"
)

func openPath(path string) error {
	command := "xdg-open"
	if runtime.GOOS == "darwin" {
		command = "open"
	}
	if err := exec.Command(command, path).Start(); err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	return nil
}
