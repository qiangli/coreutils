//go:build unix

package chrootcmd

import (
	"os/exec"
	"syscall"
)

func supportsChroot() bool { return true }

func setChroot(c *exec.Cmd, root string, cred *credentialSpec) {
	c.SysProcAttr = &syscall.SysProcAttr{Chroot: root}
	if cred != nil {
		c.SysProcAttr.Credential = &syscall.Credential{Uid: cred.uid, Gid: cred.gid, Groups: cred.groups}
	}
}
