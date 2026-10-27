//go:build !windows

package syncer

import "os/exec"

func hideWindow(*exec.Cmd) {}
