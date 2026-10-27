package syncer

import (
	"os/exec"
	"syscall"
)

// hideWindow stops ssh.exe from flashing a console window when started from a GUI app.
func hideWindow(cmd *exec.Cmd) {
	const createNoWindow = 0x08000000
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}
