//go:build unix

package nicecmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"

	"github.com/qiangli/coreutils/tool"
)

const (
	priorityHelperArg = "--coreutils-internal-nice-priority-helper"
	priorityBarrier   = "coreutils-nice-priority-barrier-v1"
)

// These seams make helper startup and priority-adjustment failures
// deterministic without changing a live test process's scheduler state.
// The production exec call does not return on success.
var (
	helperExec       = syscall.Exec
	helperExecutable = os.Executable
	prioritySetter   = setPriority
)

func init() {
	if len(os.Args) < 2 || os.Args[1] != priorityHelperArg {
		return
	}
	barrier := os.NewFile(3, "nice-priority-barrier")
	os.Exit(runPriorityHelper(os.Args[2:], os.Environ(), os.Stderr, barrier))
}

// startPriorityCommand starts a copy of the current pure-Go host which blocks
// on a pipe before overlaying itself with the requested utility. The parent
// attempts the priority change while the helper is blocked, then releases it.
// Thus utility code cannot run before the attempt, while an embedding shell's
// process priority remains untouched.
func startPriorityCommand(rc *tool.RunContext, name, path string, args []string, niceness int) (*exec.Cmd, error) {
	executable, err := helperExecutable()
	if err != nil {
		return nil, &niceStartError{fmt.Errorf("cannot locate executable: %w", err)}
	}
	barrierRead, barrierWrite, err := os.Pipe()
	if err != nil {
		return nil, &niceStartError{fmt.Errorf("cannot create priority barrier: %w", err)}
	}
	defer barrierRead.Close()
	defer barrierWrite.Close()

	ctx := rc.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	helperArgs := []string{priorityHelperArg, name, path}
	helperArgs = append(helperArgs, args...)
	c := exec.CommandContext(ctx, executable, helperArgs...)
	c.Dir = rc.Dir
	c.Env = rc.Env
	diagnostic, childErr := synchronizedStderr(rc.Err)
	c.Stdin, c.Stdout, c.Stderr = rc.In, rc.Out, childErr
	c.ExtraFiles = []*os.File{barrierRead}
	if err := c.Start(); err != nil {
		return c, &niceStartError{err}
	}
	// The helper cannot exec the utility until this attempt has completed.
	if err := prioritySetter(c.Process.Pid, niceness); err != nil {
		fmt.Fprintf(diagnostic, "%s: cannot set niceness: %v\n", name, err)
	}
	if n, err := io.WriteString(barrierWrite, priorityBarrier); err != nil || n != len(priorityBarrier) {
		if err == nil {
			err = io.ErrShortWrite
		}
		_ = c.Process.Kill()
		_ = c.Wait()
		return c, &niceStartError{fmt.Errorf("cannot release priority helper: %w", err)}
	}
	return c, nil
}

// runPriorityHelper receives: diagnostic-name, resolved utility, utility
// arguments. It waits until its parent has attempted the priority adjustment,
// then execs the utility. Adjustment failure is handled by the parent and the
// helper is still released, exactly as Issue 7 requires.
func runPriorityHelper(args, environ []string, stderr io.Writer, barrier io.Reader) int {
	if len(args) < 2 {
		fmt.Fprintln(stderr, "nice: invalid internal priority helper invocation")
		return 125
	}
	release := make([]byte, len(priorityBarrier))
	if _, err := io.ReadFull(barrier, release); err != nil {
		fmt.Fprintf(stderr, "nice: priority helper barrier failed: %v\n", err)
		return 125
	}
	if closer, ok := barrier.(io.Closer); ok {
		_ = closer.Close()
	}
	if string(release) != priorityBarrier {
		fmt.Fprintln(stderr, "nice: invalid priority helper barrier")
		return 125
	}
	name, path, utilityArgs := args[0], args[1], args[2:]
	targetArgv := append([]string{path}, utilityArgs...)
	err := helperExec(path, targetArgv, environ)
	if errors.Is(err, syscall.ENOEXEC) {
		targetArgv = append([]string{"/bin/sh", path}, utilityArgs...)
		err = helperExec("/bin/sh", targetArgv, environ)
	}
	if errors.Is(err, syscall.ENOENT) {
		fmt.Fprintf(stderr, "%s: failed to run command %q: %v\n", name, path, err)
		return 127
	}
	if err != nil {
		fmt.Fprintf(stderr, "%s: failed to run command %q: %v\n", name, path, err)
		return 126
	}
	return 0
}

type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (w *lockedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.w.Write(p)
}

// os/exec copies child output concurrently when stderr is not an *os.File.
// The parent may also emit the permitted adjustment warning, so serialize both
// paths for embedding writers such as bytes.Buffer. Real file descriptors are
// safe to share directly and retain their descriptor semantics.
func synchronizedStderr(stderr io.Writer) (diagnostic, child io.Writer) {
	if _, ok := stderr.(*os.File); ok {
		return stderr, stderr
	}
	w := &lockedWriter{w: stderr}
	return w, w
}
