//go:build !windows

package media

import (
	"os/exec"
	"syscall"
)

func setRecordProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
