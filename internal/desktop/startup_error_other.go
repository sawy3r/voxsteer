//go:build desktop && !windows

package desktop

import "log"

func ShowStartupError(err error) {
	log.Printf("VoxSteer could not start: %v", err)
}
