// Package shredcmd implements shred(1) for regular files: overwrite file
// contents to make casual recovery harder, optionally remove the file.
package shredcmd

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"unicode"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "shred",
	Synopsis: "Overwrite FILE(s) to hide their contents, and optionally delete them.",
	Usage:    "shred [OPTION]... FILE...",
}

func init() { cmd.Run = run; tool.Register(cmd) }

type shredder struct {
	rc         *tool.RunContext
	iterations int
	zero       bool
	remove     bool
	force      bool
	verbose    bool
	failed     bool
}

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	iter := fs.IntP("iterations", "n", 3, "overwrite N times instead of the default (3)")
	zero := fs.BoolP("zero", "z", false, "add a final overwrite with zeros to hide shredding")
	remove := fs.BoolP("remove", "u", false, "truncate and remove file after overwriting")
	force := fs.BoolP("force", "f", false, "change permissions to allow writing if necessary")
	verbose := fs.BoolP("verbose", "v", false, "show progress")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	if *iter < 0 {
		return tool.UsageError(rc, cmd, "invalid number of passes: '%s'", strconv.Itoa(*iter))
	}
	if len(operands) == 0 {
		return tool.UsageError(rc, cmd, "missing file operand")
	}
	s := &shredder{rc: rc, iterations: *iter, zero: *zero, remove: *remove, force: *force, verbose: *verbose}
	for _, name := range operands {
		s.shred(name)
	}
	if s.failed {
		return 1
	}
	return 0
}

func (s *shredder) shred(name string) {
	path := s.rc.Path(name)
	fi, err := os.Lstat(path)
	if err != nil {
		s.errf("failed to open '%s' for writing: %s", name, reason(err))
		return
	}
	if !fi.Mode().IsRegular() {
		s.errf("'%s': not a regular file", name)
		return
	}
	if s.force && fi.Mode().Perm()&0o200 == 0 {
		_ = os.Chmod(path, fi.Mode().Perm()|0o200)
	}
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		s.errf("failed to open '%s' for writing: %s", name, reason(err))
		return
	}
	defer f.Close()
	size := fi.Size()
	for pass := 0; pass < s.iterations; pass++ {
		if err := overwrite(f, size, rand.Reader); err != nil {
			s.errf("'%s': pass %d failed: %s", name, pass+1, reason(err))
			return
		}
		s.verbosef("%s: pass %d/%d (random)", name, pass+1, s.iterations)
	}
	if s.zero {
		if err := overwrite(f, size, zeroReader{}); err != nil {
			s.errf("'%s': zero pass failed: %s", name, reason(err))
			return
		}
		s.verbosef("%s: pass %d/%d (000000)", name, s.iterations+1, s.iterations+1)
	}
	if err := f.Sync(); err != nil {
		s.errf("'%s': fsync failed: %s", name, reason(err))
		return
	}
	if s.remove {
		if err := f.Truncate(0); err != nil {
			s.errf("'%s': truncate failed: %s", name, reason(err))
			return
		}
		if err := f.Close(); err != nil {
			s.errf("'%s': close failed: %s", name, reason(err))
			return
		}
		f = nil
		if err := os.Remove(path); err != nil {
			s.errf("'%s': remove failed: %s", name, reason(err))
			return
		}
		s.verbosef("%s: removed", name)
	}
}

func overwrite(f *os.File, size int64, src io.Reader) error {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	buf := make([]byte, 32*1024)
	var written int64
	for written < size {
		n := int64(len(buf))
		if remain := size - written; remain < n {
			n = remain
		}
		chunk := int(n)
		if _, err := io.ReadFull(src, buf[:chunk]); err != nil {
			return err
		}
		if _, err := f.Write(buf[:chunk]); err != nil {
			return err
		}
		written += n
	}
	return nil
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0
	}
	return len(p), nil
}

func (s *shredder) errf(format string, a ...any) {
	fmt.Fprintf(s.rc.Err, "shred: "+format+"\n", a...)
	s.failed = true
}

func (s *shredder) verbosef(format string, a ...any) {
	if s.verbose {
		fmt.Fprintf(s.rc.Err, "shred: "+format+"\n", a...)
	}
}

func reason(err error) string {
	var pe *os.PathError
	if errors.As(err, &pe) {
		err = pe.Err
	}
	var se *os.SyscallError
	if errors.As(err, &se) {
		err = se.Err
	}
	text := err.Error()
	if text == "" {
		return text
	}
	r := []rune(text)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}
