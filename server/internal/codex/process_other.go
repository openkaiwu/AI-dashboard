//go:build !windows

package codex

import "os/exec"

func hideWindow(cmd *exec.Cmd) {}
