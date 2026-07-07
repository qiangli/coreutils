// Package mktempcmd implements mktemp(1) per the GNU coreutils manual:
// create a temporary file or directory safely and print its name.
//
// Portions adapted from https://github.com/u-root/u-root cmds/core/mktemp (BSD-3-Clause).
// Changes: rewired to tool framework; template X-run substitution
// implemented per GNU (last run of >=3 consecutive X's in the final
// component, implied suffix after it) instead of os.CreateTemp prefix
// splitting; $TMPDIR read from rc.Env, never the process environment.
package mktempcmd

import (
	cryptorand "crypto/rand"
	"errors"
	"fmt"
	iofs "io/fs"
	"os"
	"path/filepath"
	"runtime"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "mktemp",
	Synopsis: "Create a temporary file or directory, safely, and print its name.",
	Usage:    "mktemp [OPTION]... [TEMPLATE]",
}

// Run is wired in init: a literal would create an initialization cycle.
func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	directory := fs.BoolP("directory", "d", false, "create a directory, not a file")
	dryRun := fs.BoolP("dry-run", "u", false, "do not create anything; merely print a name (unsafe)")
	quiet := fs.BoolP("quiet", "q", false, "suppress diagnostics about file creation failure")
	useTmp := fs.BoolP("tmpdir-template", "t", false, "interpret TEMPLATE relative to the temporary directory")
	suffix := fs.String("suffix", "", "append SUFFIX to TEMPLATE")
	// GNU: -p DIR, --tmpdir[=DIR]. The optional-argument long form
	// ("--tmpdir" alone meaning $TMPDIR-else-/tmp) is not expressible
	// here; a bare --tmpdir fails with a needs-an-argument error.
	tmpdir := fs.StringP("tmpdir", "p", "", "interpret TEMPLATE relative to DIR; TEMPLATE must not be absolute")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}

	template := "tmp.XXXXXXXXXX"
	explicit := false
	switch len(operands) {
	case 0:
	case 1:
		template = operands[0]
		explicit = true
	default:
		return tool.UsageError(rc, cmd, "too many templates")
	}

	base := ""
	switch {
	case *tmpdir != "":
		if filepath.IsAbs(template) {
			if !*quiet {
				fmt.Fprintf(rc.Err, "mktemp: invalid template, '%s'; with --tmpdir, it may not be absolute\n", template)
			}
			return 1
		}
		base = *tmpdir
	case *useTmp:
		if filepath.IsAbs(template) {
			if !*quiet {
				fmt.Fprintf(rc.Err, "mktemp: invalid template, '%s'; with -t, it may not be absolute\n", template)
			}
			return 1
		}
		base = defaultTmpDir(rc)
	case !explicit:
		// The default template implies --tmpdir.
		base = defaultTmpDir(rc)
	}

	printed := template
	if base != "" {
		printed = filepath.Join(base, template)
	}

	dir, file := filepath.Split(printed)
	// GNU substitutes the last run of consecutive X's in the final
	// component (text after it is the implied --suffix) and requires at
	// least three of them.
	runStart, runEnd := -1, -1
	for i := 0; i < len(file); i++ {
		if file[i] == 'X' {
			j := i
			for j < len(file) && file[j] == 'X' {
				j++
			}
			runStart, runEnd = i, j
			i = j
		}
	}
	if runStart < 0 || runEnd-runStart < 3 {
		if !*quiet {
			fmt.Fprintf(rc.Err, "mktemp: too few X's in template '%s'\n", template)
		}
		return 1
	}

	kind := "file"
	if *directory {
		kind = "directory"
	}
	var lastErr error
	for attempt := 0; attempt < 100; attempt++ {
		random, err := randomLetters(runEnd - runStart)
		if err != nil {
			lastErr = err
			break
		}
		name := dir + file[:runStart] + random + file[runEnd:] + *suffix
		if *dryRun {
			fmt.Fprintln(rc.Out, name)
			return 0
		}
		path := rc.Path(name)
		if *directory {
			err = os.Mkdir(path, 0o700)
		} else {
			var f *os.File
			f, err = os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
			if err == nil {
				f.Close()
			}
		}
		if err == nil {
			fmt.Fprintln(rc.Out, name)
			return 0
		}
		lastErr = err
		if !errors.Is(err, iofs.ErrExist) {
			break
		}
	}
	if !*quiet {
		fmt.Fprintf(rc.Err, "mktemp: failed to create %s via template '%s': %v\n", kind, template, reason(lastErr))
	}
	return 1
}

// defaultTmpDir resolves the implied --tmpdir from the invocation
// environment, never the process's: $TMPDIR, else the platform default.
func defaultTmpDir(rc *tool.RunContext) string {
	if d := rc.Getenv("TMPDIR"); d != "" {
		return d
	}
	if runtime.GOOS == "windows" {
		for _, key := range []string{"TMP", "TEMP"} {
			if d := rc.Getenv(key); d != "" {
				return d
			}
		}
		if d := rc.Getenv("USERPROFILE"); d != "" {
			return filepath.Join(d, "AppData", "Local", "Temp")
		}
		return `C:\Windows\Temp`
	}
	return "/tmp"
}

const letters = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

func randomLetters(n int) (string, error) {
	b := make([]byte, n)
	if _, err := cryptorand.Read(b); err != nil {
		return "", err
	}
	out := make([]byte, n)
	for i, v := range b {
		out[i] = letters[int(v)%len(letters)]
	}
	return string(out), nil
}

// reason unwraps os wrapper errors so diagnostics read like GNU's.
func reason(err error) error {
	var pe *os.PathError
	if errors.As(err, &pe) {
		return pe.Err
	}
	if err == nil {
		return errors.New("unknown error")
	}
	return err
}
