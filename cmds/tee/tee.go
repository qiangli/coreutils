// Package teecmd implements tee(1) per the GNU coreutils manual: copy
// standard input to each FILE, and also to standard output.
//
// Portions adapted from https://github.com/u-root/u-root
// cmds/core/tee/tee.go (BSD-3-Clause).
// Changes: rewired to the tool framework; per-file open and write
// errors are diagnosed and skipped with exit status 1 (GNU behavior)
// instead of aborting the whole copy; -i is accepted as a documented
// no-op (no process signals exist in this in-process userland).
package teecmd

import (
	"errors"
	"fmt"
	"io"
	"os"

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
	fs := tool.NewFlags(cmd.Name)
	appendMode := fs.BoolP("append", "a", false, "append to the given FILEs, do not overwrite")
	fs.BoolP("ignore-interrupts", "i", false, "ignore interrupt signals")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	// -i is accepted for upstream compatibility but has no effect here:
	// tools run in-process and never receive a SIGINT of their own to
	// ignore, which matches the documented effect for this embedding.

	oflags := os.O_WRONLY | os.O_CREATE
	if *appendMode {
		oflags |= os.O_APPEND
	} else {
		oflags |= os.O_TRUNC
	}

	status := 0
	type sink struct {
		name string
		w    io.Writer
		f    *os.File
	}
	sinks := []sink{{name: "standard output", w: rc.Out}}
	for _, name := range operands {
		f, err := os.OpenFile(rc.Path(name), oflags, 0o666)
		if err != nil {
			fmt.Fprintf(rc.Err, "tee: %s: %v\n", name, pathErr(err))
			status = 1
			continue
		}
		sinks = append(sinks, sink{name: name, w: f, f: f})
	}

	buf := make([]byte, 32*1024)
	for {
		n, rerr := rc.In.Read(buf)
		if n > 0 {
			for i := range sinks {
				if sinks[i].w == nil {
					continue // already failed; keep copying to the rest
				}
				if _, werr := sinks[i].w.Write(buf[:n]); werr != nil {
					fmt.Fprintf(rc.Err, "tee: %s: %v\n", sinks[i].name, pathErr(werr))
					sinks[i].w = nil
					status = 1
				}
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
	return status
}

// pathErr unwraps *fs.PathError so diagnostics read "tee: f: no such
// file or directory" instead of repeating the operation and path.
func pathErr(err error) string {
	return tool.SysErrString(err)
}
