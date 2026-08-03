//go:build unix

package killcmd

import "os"

func currentPID() int { return os.Getpid() }
