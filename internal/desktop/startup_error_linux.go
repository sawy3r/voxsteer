//go:build desktop && linux

package desktop

import (
	"log"
	"os/exec"
)

// ShowStartupError records the failure and raises a desktop notification so
// the problem is visible even when VoxSteer was launched from an autostart
// entry with no terminal attached.
func ShowStartupError(err error) {
	log.Printf("VoxSteer could not start: %v", err)
	if path, lookErr := exec.LookPath("notify-send"); lookErr == nil {
		_ = exec.Command(path, "--app-name=VoxSteer", "--urgency=critical",
			"VoxSteer could not start", err.Error()).Run()
	}
}
