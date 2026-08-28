//go:build !windows

package chat

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

// startParentDeathWatch puts a tiny watcher in the child's process group. If
// the meet process is killed (including SIGKILL, which cannot be handled), the
// watcher notices that the recorded parent PID is gone and kills the group.
// This is needed on macOS as well as Linux: unlike Linux, Darwin has no
// portable Go SysProcAttr parent-death signal.
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
	// Keep the watcher out of the participant group. It must survive long
	// enough to observe the participant's parent becoming a zombie, then signal
	// the participant group and exit itself. Normal cancellation stops it.
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
	// Kill first so normal teardown cannot race the EOF handler into signalling
	// a reused process group. Close the writer as well: it is the only resource
	// that keeps the watcher blocked, and must not survive this call.
	_ = watch.cmd.Process.Kill()
	_ = watch.retain.Close()
	_ = watch.cmd.Wait()
}
