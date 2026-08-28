//go:build !windows

package agentpty

import (
	"io"
	"os"
	"os/exec"
	"strconv"
	"syscall"
)

type parentDeathWatch struct {
	cmd    *exec.Cmd
	retain *os.File
}

func startParentDeathWatch(childPID int) *parentDeathWatch {
	reader, writer, err := os.Pipe()
	if err != nil {
		return nil
	}
	watch := exec.Command("sh", "-c", `
IFS= read -r _ <&3 || true
group=$1
kill -KILL -"$group" 2>/dev/null || true
	`, "bashy-parent-watch", strconv.Itoa(childPID))
	watch.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	watch.ExtraFiles = []*os.File{reader}
	watch.Stdout = io.Discard
	watch.Stderr = io.Discard
	if err := watch.Start(); err != nil {
		_ = reader.Close()
		_ = writer.Close()
		return nil
	}
	_ = reader.Close()
	return &parentDeathWatch{cmd: watch, retain: writer}
}

func stopParentDeathWatch(watch *parentDeathWatch) {
	if watch == nil || watch.cmd == nil || watch.cmd.Process == nil {
		return
	}
	_ = watch.cmd.Process.Kill()
	_ = watch.retain.Close()
	_ = watch.cmd.Wait()
}
