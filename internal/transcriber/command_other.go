//go:build !windows

package transcriber

import "os/exec"

func configureCommand(*exec.Cmd) {}
