//go:build desktop && !windows && !linux

package desktop

import (
	"log"
	"os/exec"
)

// ShowStartupError records the failure and raises an alert. VoxSteer.app has no
// window or terminal, and a notification would truncate the setup instructions
// these errors carry.
func ShowStartupError(err error) {
	log.Printf("VoxSteer could not start: %v", err)
	script := `on run argv
display alert "VoxSteer could not start" message (item 1 of argv) as critical
end run`
	_ = exec.Command("/usr/bin/osascript", "-e", script, err.Error()).Run()
}
