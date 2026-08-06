//go:build darwin

package main

import "os/exec"

func showStartupError(startupErr error) {
	script := `on run argv
display alert "VoxSteer Browser could not start" message (item 1 of argv) as critical
end run`
	_ = exec.Command("/usr/bin/osascript", "-e", script, startupErr.Error()).Run()
}
