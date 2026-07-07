//go:build !unix

package chrootcmd

import "os/exec"

func supportsChroot() bool { return false }

func setChroot(c *exec.Cmd, root string, cred *credentialSpec) {}
