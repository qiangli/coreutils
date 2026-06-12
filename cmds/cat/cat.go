// Package catcmd implements cat(1) per the GNU coreutils manual:
// concatenate FILE(s) to standard output.
//
// Portions adapted from https://github.com/guonaihong/coreutils cat/cat.go (Apache-2.0).
// Changes: rewired to tool framework; byte-wise transform instead of a
// strings.Replacer; GNU-exact -b/-s blank-line rule (empty line, not
// whitespace-only); ^M$ rendering for CRLF under -E; numbering and squeeze
// state carried across files; -e/-t/-u pre-parsed (no long forms upstream).
package catcmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "cat",
	Synopsis: "Concatenate FILE(s) to standard output.\nWith no FILE, or when FILE is -, read standard input.\nAlso accepts the short-only GNU flags: -e (same as -vE), -t (same as -vT), -u (ignored).",
	Usage:    "cat [OPTION]... [FILE]...",
}

func init() { cmd.Run = run; tool.Register(cmd) }

type catOpts struct {
	numberAll      bool // -n
	numberNonblank bool // -b (overrides -n)
	squeeze        bool // -s
	showEnds       bool // -E
	showTabs       bool // -T
	showNP         bool // -v
}

// catState persists across files: GNU cat numbers lines and squeezes
// blank runs across the whole concatenated output.
type catState struct {
	lineNo   int64
	blankRun int
}

func run(rc *tool.RunContext, args []string) int {
	args, optE, optT, optU := extractShortOnly(args)
	_ = optU // -u is ignored, per the GNU manual

	fs := tool.NewFlags(cmd.Name)
	showAll := fs.BoolP("show-all", "A", false, "equivalent to -vET")
	numberNonblank := fs.BoolP("number-nonblank", "b", false, "number nonempty output lines, overrides -n")
	showEnds := fs.BoolP("show-ends", "E", false, "display $ at end of each line")
	number := fs.BoolP("number", "n", false, "number all output lines")
	squeeze := fs.BoolP("squeeze-blank", "s", false, "suppress repeated empty output lines")
	showTabs := fs.BoolP("show-tabs", "T", false, "display TAB characters as ^I")
	showNP := fs.BoolP("show-nonprinting", "v", false, "use ^ and M- notation, except for LFD and TAB")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}

	o := catOpts{
		numberAll:      *number,
		numberNonblank: *numberNonblank,
		squeeze:        *squeeze,
		showEnds:       *showEnds,
		showTabs:       *showTabs,
		showNP:         *showNP,
	}
	if *showAll {
		o.showNP, o.showEnds, o.showTabs = true, true, true
	}
	if optE {
		o.showNP, o.showEnds = true, true
	}
	if optT {
		o.showNP, o.showTabs = true, true
	}
	if o.numberNonblank {
		o.numberAll = false
	}

	files := operands
	if len(files) == 0 {
		files = []string{"-"}
	}

	w := bufio.NewWriter(rc.Out)
	st := &catState{}
	exit := 0
	for _, name := range files {
		var r io.Reader
		var closer io.Closer
		if name == "-" {
			r = rc.In
			if r == nil {
				r = strings.NewReader("")
			}
		} else {
			f, err := os.Open(rc.Path(name))
			if err != nil {
				fmt.Fprintf(rc.Err, "cat: %s: %v\n", name, sysErr(err))
				exit = 1
				continue
			}
			r = f
			closer = f
		}
		if err := catStream(r, w, o, st); err != nil {
			fmt.Fprintf(rc.Err, "cat: %s: %v\n", name, sysErr(err))
			exit = 1
		}
		if closer != nil {
			closer.Close()
		}
	}
	if err := w.Flush(); err != nil {
		fmt.Fprintf(rc.Err, "cat: write error: %v\n", err)
		return 1
	}
	return exit
}

// extractShortOnly removes the short-only GNU flags -e, -t, -u from
// args (they have no long forms upstream, so they cannot be defined on
// the pflag set without inventing names). cat has no value-taking short
// flags, so every "-xyz" cluster is a pure flag cluster.
func extractShortOnly(args []string) (out []string, e, t, u bool) {
	out = make([]string, 0, len(args))
	rest := false
	for _, a := range args {
		if rest || a == "--" || a == "-" || !strings.HasPrefix(a, "-") || strings.HasPrefix(a, "--") {
			if a == "--" {
				rest = true
			}
			out = append(out, a)
			continue
		}
		kept := []byte{'-'}
		for i := 1; i < len(a); i++ {
			switch a[i] {
			case 'e':
				e = true
			case 't':
				t = true
			case 'u':
				u = true
			default:
				kept = append(kept, a[i])
			}
		}
		if len(kept) > 1 {
			out = append(out, string(kept))
		}
	}
	return out, e, t, u
}

func catStream(r io.Reader, w *bufio.Writer, o catOpts, st *catState) error {
	br := bufio.NewReader(r)
	if o == (catOpts{}) {
		_, err := io.Copy(w, br)
		return err
	}
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			emitLine(w, line, o, st)
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func emitLine(w *bufio.Writer, line []byte, o catOpts, st *catState) {
	hasNL := line[len(line)-1] == '\n'
	content := line
	if hasNL {
		content = line[:len(line)-1]
	}
	blank := hasNL && len(content) == 0

	if o.squeeze {
		if blank {
			st.blankRun++
			if st.blankRun > 1 {
				return
			}
		} else {
			st.blankRun = 0
		}
	}

	if o.numberNonblank {
		if !blank {
			st.lineNo++
			fmt.Fprintf(w, "%6d\t", st.lineNo)
		}
	} else if o.numberAll {
		st.lineNo++
		fmt.Fprintf(w, "%6d\t", st.lineNo)
	}

	end := len(content)
	trailCR := false
	// GNU manual on -E: "the \r\n combination is shown as ^M$". With -v
	// the \r is already rendered as ^M by the nonprinting transform.
	if o.showEnds && !o.showNP && hasNL && end > 0 && content[end-1] == '\r' {
		trailCR = true
		end--
	}
	for i := 0; i < end; i++ {
		writeTransformed(w, content[i], o)
	}
	if trailCR {
		w.WriteString("^M")
	}
	if hasNL {
		if o.showEnds {
			w.WriteByte('$')
		}
		w.WriteByte('\n')
	}
}

// writeTransformed renders one byte with GNU cat's ^ / M- notation.
// Mapping adapted from guonaihong/coreutils cat (writeNonblank).
func writeTransformed(w *bufio.Writer, c byte, o catOpts) {
	if c == '\t' {
		if o.showTabs {
			w.WriteString("^I")
		} else {
			w.WriteByte(c)
		}
		return
	}
	if !o.showNP {
		w.WriteByte(c)
		return
	}
	switch {
	case c < 32:
		w.WriteByte('^')
		w.WriteByte(c + 64)
	case c < 127:
		w.WriteByte(c)
	case c == 127:
		w.WriteString("^?")
	case c < 160:
		w.WriteString("M-^")
		w.WriteByte(c - 64)
	case c < 255:
		w.WriteString("M-")
		w.WriteByte(c - 128)
	default:
		w.WriteString("M-^?")
	}
}

// sysErr unwraps *fs.PathError so messages read "cat: NAME: no such
// file or directory" (GNU shape) instead of duplicating the path.
func sysErr(err error) error {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return pe.Err
	}
	return err
}
