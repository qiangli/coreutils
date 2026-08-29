// Package teecmd implements tee(1) per the GNU coreutils manual: copy
// standard input to each FILE, and also to standard output.
//
// Portions adapted from https://github.com/u-root/u-root
// cmds/core/tee/tee.go (BSD-3-Clause).
// Changes: rewired to the tool framework; per-file open and write
// errors are diagnosed and skipped with exit status 1 (GNU behavior)
// instead of aborting the whole copy. POSIX default behavior makes
// standard-output write errors fatal and diagnosed, while GNU
// --output-error modes (and the -p shorthand) diagnose or exit on
// non-pipe errors and ignore pipe errors. -i ignores interrupt signals.
package teecmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "tee",
	Synopsis: "Copy standard input to each FILE, and also to standard output.",
	Usage:    "tee [OPTION]... [FILE]...",
}

// Run is wired in init: a literal would create an initialization
// cycle (run's flag-error paths reference cmd).
func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	return runWithOpen(rc, args, func(path string, flags int, perm os.FileMode) (io.WriteCloser, error) {
		return rc.OpenFile(path, flags, perm)
	})
}

type openOutputFunc func(path string, flags int, perm os.FileMode) (io.WriteCloser, error)

func runWithOpen(rc *tool.RunContext, args []string, openOutput openOutputFunc) int {
	fs := tool.NewFlags(cmd.Name)
	appendMode := fs.BoolP("append", "a", false, "append to the given FILEs, do not overwrite")
	ignoreInterrupts := fs.BoolP("ignore-interrupts", "i", false, "ignore interrupt signals")
	ignorePipeErrors := fs.BoolP("ignore-pipe-errors", "p", false, "diagnose errors writing to non pipe outputs")
	outputError := fs.String("output-error", "", "set behavior on write error: warn, warn-nopipe, exit, or exit-nopipe")
	fs.Lookup("output-error").NoOptDefVal = "warn-nopipe"
	// Issue 7's utility-syntax contract ends option recognition at the
	// first operand. In particular, a lone "-" is an ordinary output-file
	// operand; later option-shaped arguments are therefore pathnames too.
	operands, code := tool.ParseRequireOrder(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	if *ignoreInterrupts {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		defer signal.Stop(c)
	}

	if *ignorePipeErrors && !fs.Changed("output-error") {
		*outputError = "warn-nopipe"
	}
	mode := outputErrorMode(strings.ToLower(*outputError))
	switch mode {
	case "", modeWarn, modeWarnNoPipe, modeExit, modeExitNoPipe:
	default:
		return tool.UsageError(rc, cmd, "invalid argument %q for --output-error", *outputError)
	}
	if mode == "" {
		// POSIX default: file errors are diagnosed and tee continues;
		// stdout errors are diagnosed and fatal.
		mode = modePOSIX
	}

	oflags := os.O_WRONLY | os.O_CREATE
	if *appendMode {
		oflags |= os.O_APPEND
	} else {
		oflags |= os.O_TRUNC
	}

	status := 0
	type sink struct {
		name     string
		w        io.Writer
		f        io.Closer
		isStdout bool
	}
	sinks := []sink{{name: "standard output", w: rc.Out, isStdout: true}}
	for _, name := range operands {
		f, err := openOutput(rc.Path(name), oflags, 0o666)
		if err != nil {
			fmt.Fprintf(rc.Err, "tee: %s: %v\n", name, pathErr(err))
			status = 1
			continue
		}
		sinks = append(sinks, sink{name: name, w: f, f: f})
	}

	buf := make([]byte, 32*1024)
	writeErr := false
Write:
	for {
		n, rerr := rc.In.Read(buf)
		if n > 0 {
			for i := range sinks {
				s := &sinks[i]
				if s.w == nil {
					continue // already failed; keep copying to the rest
				}
				wn, werr := s.w.Write(buf[:n])
				if werr == nil && wn != n {
					werr = io.ErrShortWrite
				}
				if werr == nil {
					continue
				}
				isPipe := isPipeWriter(s.w)
				diagnose, exit := mode.writeBehavior(s.isStdout, isPipe)
				if diagnose {
					fmt.Fprintf(rc.Err, "tee: %s: %v\n", s.name, pathErr(werr))
					status = 1
				}
				if exit {
					s.w = nil
					writeErr = true
					break Write
				}
				// Stop hammering a sink that just errored, whether the
				// error was diagnosed (warn/exit/POSIX file) or ignored
				// (pipe in *-nopipe mode).
				s.w = nil
			}
		}
		if rerr != nil {
			if !errors.Is(rerr, io.EOF) {
				fmt.Fprintf(rc.Err, "tee: read error: %v\n", rerr)
				status = 1
			}
			break
		}
	}

	for _, s := range sinks {
		if s.f == nil {
			continue
		}
		if err := s.f.Close(); err != nil && s.w != nil {
			fmt.Fprintf(rc.Err, "tee: %s: %v\n", s.name, pathErr(err))
			status = 1
		}
	}
	if status == 0 && writeErr {
		status = 1
	}
	return status
}

// pathErr unwraps *fs.PathError so diagnostics read "tee: f: no such
// file or directory" instead of repeating the operation and path.
func pathErr(err error) string {
	return tool.SysErrString(err)
}

// outputErrorMode controls how tee reacts to write errors. The empty
// string selects POSIX default behavior; the other names mirror GNU
// coreutils --output-error modes.
type outputErrorMode string

const (
	modePOSIX      outputErrorMode = "posix"
	modeWarn       outputErrorMode = "warn"
	modeWarnNoPipe outputErrorMode = "warn-nopipe"
	modeExit       outputErrorMode = "exit"
	modeExitNoPipe outputErrorMode = "exit-nopipe"
)

// writeBehavior reports, for a write error to stdout or a file operand,
// whether the error should be diagnosed and whether tee should stop.
// POSIX treats stdout specially: write errors there are diagnosed and fatal.
func (m outputErrorMode) writeBehavior(isStdout, isPipe bool) (diagnose, exit bool) {
	if isStdout && m == modePOSIX {
		// POSIX gives only successfully opened file operands the special
		// continue-after-write-error rule. Other errors use the Utility
		// Description Defaults: diagnose the error and fail.
		return true, true
	}
	switch m {
	case modePOSIX:
		// POSIX: diagnose file errors, keep copying to remaining sinks.
		return true, false
	case modeWarn:
		return true, false
	case modeWarnNoPipe:
		return !isPipe, false
	case modeExit:
		return true, true
	case modeExitNoPipe:
		return !isPipe, !isPipe
	}
	return true, false
}

// pipeMarker lets command-local tests mark an io.Writer as a pipe without
// needing an actual OS pipe.
type pipeMarker interface {
	isPipe() bool
}

// isPipeWriter reports whether w is known to be a pipe. Real *os.File
// sinks are classified by their stat mode; test doubles can implement
// pipeMarker to simulate pipe behavior portably.
func isPipeWriter(w io.Writer) bool {
	if pm, ok := w.(pipeMarker); ok {
		return pm.isPipe()
	}
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeNamedPipe != 0
}
