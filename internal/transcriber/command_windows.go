//go:build windows

package transcriber

import (
	"os/exec"
	"syscall"
)

const createNoWindow = 0x08000000

func configureCommand(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
}
