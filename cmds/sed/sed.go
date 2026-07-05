// Package sedcmd implements a pure-Go drop-in for GNU sed: a stream editor that
// applies a script to each line of its input.
//
// The script engine is the vendored Go.Sed (MIT — see internal/gosed/LICENSE),
// adapted for GNU compatibility: patterns default to POSIX Basic Regular
// Expressions (BRE), switching to ERE under -E/-r, via coreutils/pkg/bre (the
// same translator grep uses); s/// replacements use GNU `\1`/`&` form. The full
// command set is supported — s, y, d, D, p, P, n, N, g, G, h, H, x, b, t,
// :label, a, i, c, r, w, q, = and address ranges. Constructs RE2 cannot express
// (back-references \1..\9 in a PATTERN, \< \> word anchors) fail loudly rather
// than mis-match.
//
// Flags: -n (suppress auto-print), -e SCRIPT, -f FILE, -E/-r (ERE), -s (treat
// files separately), -i[SUFFIX] (edit in place). Unsupported flags fail loudly.
package sedcmd

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/qiangli/coreutils/cmds/sed/internal/gosed"
	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "sed",
	Synopsis: "Stream editor: apply a sed script to each line of input (GNU sed drop-in).",
	Usage:    "sed [-nErs] [-e SCRIPT]... [-f FILE]... [-i[SUFFIX]] [SCRIPT] [FILE...]",
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	// GNU's -i takes an OPTIONAL attached suffix (`-i.bak`), which getopt-style
	// flag parsers don't model. Pre-strip that form so pflag only sees bare -i /
	// --in-place[=SUFFIX].
	var inPlacePre bool
	var inPlaceSuffix string
	{
		filtered := args[:0:0]
		for _, a := range args {
			if len(a) > 2 && strings.HasPrefix(a, "-i") && !strings.HasPrefix(a, "--") {
				inPlacePre, inPlaceSuffix = true, a[2:]
				continue
			}
			filtered = append(filtered, a)
		}
		args = filtered
	}

	fs := tool.NewFlags(cmd.Name)
	quiet := fs.BoolP("quiet", "n", false, "suppress automatic printing of pattern space")
	fs.BoolVar(quiet, "silent", false, "same as -n")
	var scripts []string
	fs.StringArrayVarP(&scripts, "expression", "e", nil, "add SCRIPT to the commands to be executed")
	var scriptFiles []string
	fs.StringArrayVarP(&scriptFiles, "file", "f", nil, "add the contents of FILE to the commands")
	ereE := fs.BoolP("regexp-extended", "E", false, "use extended regular expressions")
	ereR := fs.BoolP("regexp-extended-r", "r", false, "same as -E")
	separate := fs.BoolP("separate", "s", false, "consider files as separate rather than one continuous stream")
	// -i takes an optional suffix; pflag NoOptDefVal lets `-i` work without a value.
	inPlace := fs.StringP("in-place", "i", "", "edit files in place (optional backup SUFFIX)")
	fs.Lookup("in-place").NoOptDefVal = "\x00" // sentinel: -i given with no suffix

	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}

	// Assemble the program: -e / -f in order; else the first operand is the script.
	var program string
	switch {
	case len(scripts) > 0 || len(scriptFiles) > 0:
		var parts []string
		parts = append(parts, scripts...)
		for _, f := range scriptFiles {
			b, err := os.ReadFile(rc.Path(f))
			if err != nil {
				fmt.Fprintf(rc.Err, "sed: %s: %v\n", f, err)
				return 2
			}
			parts = append(parts, string(b))
		}
		program = strings.Join(parts, "\n")
	case len(operands) > 0:
		program = operands[0]
		operands = operands[1:]
	default:
		return tool.UsageError(rc, cmd, "no script specified")
	}

	gosed.ExtendedRegex = *ereE || *ereR

	inPlaceFlag := inPlacePre || fs.Lookup("in-place").Changed
	files := operands

	// In-place editing requires real files; rewrite each independently.
	if inPlaceFlag {
		if len(files) == 0 {
			return tool.UsageError(rc, cmd, "-i may not be used with stdin")
		}
		suffix := inPlaceSuffix
		if !inPlacePre {
			suffix = *inPlace
			if suffix == "\x00" {
				suffix = ""
			}
		}
		rc2 := 0
		for _, f := range files {
			if err := editInPlace(rc, program, *quiet, f, suffix); err != nil {
				fmt.Fprintf(rc.Err, "sed: %s: %v\n", f, err)
				rc2 = 2
			}
		}
		return rc2
	}

	// Stream mode: stdin, or files concatenated (one stream) / separate (-s).
	if len(files) == 0 {
		if err := apply(program, *quiet, rc.In, rc.Out); err != nil {
			fmt.Fprintf(rc.Err, "sed: %v\n", err)
			return 2
		}
		return 0
	}

	status := 0
	if *separate {
		for _, f := range files {
			r, err := openInput(rc, f)
			if err != nil {
				fmt.Fprintf(rc.Err, "sed: %s: %v\n", f, err)
				status = 2
				continue
			}
			err = apply(program, *quiet, r, rc.Out)
			closeIf(r)
			if err != nil {
				fmt.Fprintf(rc.Err, "sed: %v\n", err)
				status = 2
			}
		}
		return status
	}

	// One continuous stream across all files.
	var readers []io.Reader
	var closers []io.Closer
	for _, f := range files {
		r, err := openInput(rc, f)
		if err != nil {
			fmt.Fprintf(rc.Err, "sed: %s: %v\n", f, err)
			status = 2
			continue
		}
		readers = append(readers, r)
		if c, ok := r.(io.Closer); ok && f != "-" {
			closers = append(closers, c)
		}
	}
	if err := apply(program, *quiet, io.MultiReader(readers...), rc.Out); err != nil {
		fmt.Fprintf(rc.Err, "sed: %v\n", err)
		status = 2
	}
	for _, c := range closers {
		c.Close()
	}
	return status
}

// apply compiles the program and streams input→output through the engine.
func apply(program string, quiet bool, in io.Reader, out io.Writer) error {
	if fast, err := gosed.NewFastSubstitution(program, quiet); err != nil {
		return err
	} else if fast != nil {
		return fast.Run(in, out)
	}

	eng, err := newEngine(program, quiet)
	if err != nil {
		return err
	}
	bw := bufio.NewWriter(out)
	if _, err := io.Copy(bw, eng.Wrap(in)); err != nil {
		return err
	}
	return bw.Flush()
}

func editInPlace(rc *tool.RunContext, program string, quiet bool, file, suffix string) error {
	path := rc.Path(file)
	src, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	eng, err := newEngine(program, quiet)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, eng.Wrap(bytes.NewReader(src))); err != nil {
		return err
	}
	if suffix != "" {
		backup := path + suffix
		if strings.Contains(suffix, "*") {
			backup = strings.ReplaceAll(suffix, "*", path)
		}
		if err := os.WriteFile(backup, src, 0o644); err != nil {
			return err
		}
	}
	fi, _ := os.Stat(path)
	mode := os.FileMode(0o644)
	if fi != nil {
		mode = fi.Mode()
	}
	return os.WriteFile(path, buf.Bytes(), mode)
}

func newEngine(program string, quiet bool) (*gosed.Engine, error) {
	if quiet {
		return gosed.NewQuiet(strings.NewReader(program))
	}
	return gosed.New(strings.NewReader(program))
}

func openInput(rc *tool.RunContext, f string) (io.Reader, error) {
	if f == "-" {
		return rc.In, nil
	}
	return os.Open(rc.Path(f))
}

func closeIf(r io.Reader) {
	if c, ok := r.(io.Closer); ok {
		c.Close()
	}
}
