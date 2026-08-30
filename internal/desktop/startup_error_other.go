//go:build desktop && !windows && !linux

package desktop

import (
	"fmt"
	"log"
	"os/exec"
)

// ShowStartupError records the failure and raises a notification so the
// problem is visible even when VoxSteer was launched without a terminal.
func ShowStartupError(err error) {
	log.Printf("VoxSteer could not start: %v", err)
	if path, lookErr := exec.LookPath("osascript"); lookErr == nil {
		script := fmt.Sprintf("display notification %q with title %q",
			"VoxSteer could not start: "+err.Error(), "VoxSteer")
		_ = exec.Command(path, "-e", script).Run()
	}
}
