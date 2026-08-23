//go:build unix

package writecmd

import (
	"io/fs"
	"os"
	"path/filepath"
	"syscall"

	"github.com/qiangli/coreutils/tool"
)

// defaultSenderTTY names the sender's own terminal, as the banner reports it
// and as the self-write guard compares it. It returns "" when the caller has
// no terminal at all (a pipeline, a cron job, an agent harness), which is not
// an error: write is still useful from a script.
//
// The streams are examined in the order in/out/err so that a caller who
// redirected only one of them is still identified by another. rc's streams win
// over the process's: an embedding shell routinely hands a tool something other
// than the process's own file descriptors, and the process's would name the
// wrong device.
func defaultSenderTTY(rc *tool.RunContext) string {
	var candidates []*os.File
	if rc != nil {
		for _, s := range []any{rc.In, rc.Out, rc.Err} {
			if f, ok := s.(*os.File); ok {
				candidates = append(candidates, f)
			}
		}
	}
	candidates = append(candidates, os.Stdin, os.Stdout, os.Stderr)

	for _, f := range candidates {
		if f == nil {
			continue
		}
		fi, err := f.Stat()
		if err != nil || fi.Mode()&os.ModeCharDevice == 0 {
			continue
		}
		if name := deviceNameOf(fi); name != "" {
			return name
		}
	}
	return ""
}

// deviceNameOf maps an open character device onto its name under devDir by
// matching device numbers. Go exposes no portable ttyname(3), and assuming
// /dev/tty would name the controlling terminal even when the caller's streams
// point somewhere else entirely. Same approach as cmds/mesg's ttyPath.
func deviceNameOf(want fs.FileInfo) string {
	wantDev, ok := rdevOf(want)
	if !ok {
		return ""
	}
	for _, sub := range []string{"pts", ""} {
		dir := filepath.Join(devDir, sub)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			info, err := e.Info()
			if err != nil || info.Mode()&os.ModeCharDevice == 0 {
				continue
			}
			if dev, ok := rdevOf(info); ok && dev == wantDev {
				return filepath.Join(sub, e.Name())
			}
		}
	}
	return ""
}

func rdevOf(fi fs.FileInfo) (uint64, bool) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return uint64(st.Rdev), true
}
