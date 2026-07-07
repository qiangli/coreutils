// Package unexpandcmd implements unexpand(1): convert spaces to tabs.
package unexpandcmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "unexpand",
	Synopsis: "Convert spaces in each FILE to tabs, writing to standard output.\nBy default only leading blanks are converted. With -a, convert all blanks.\nWith no FILE, or when FILE is -, read standard input.",
	Usage:    "unexpand [OPTION]... [FILE]...",
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	tabsValue := fs.StringP("tabs", "t", "8", "use comma-separated tab stops instead of 8")
	all := fs.BoolP("all", "a", false, "convert all blanks instead of only leading blanks")
	firstOnly := fs.BoolP("first-only", "f", false, "convert only leading blanks, overriding -a")
	noUTF8 := fs.BoolP("no-utf8", "U", false, "interpret input as bytes rather than UTF-8")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	tabs, err := parseTabStops(*tabsValue)
	if err != nil {
		return tool.UsageError(rc, cmd, "invalid tab size: %q", *tabsValue)
	}
	if len(operands) == 0 {
		operands = []string{"-"}
	}
	convertAll := *all || fs.Changed("tabs")
	if *firstOnly {
		convertAll = false
	}

	out := bufio.NewWriter(rc.Out)
	status := 0
	for _, name := range operands {
		r, closer, err := openInput(rc, name)
		if err != nil {
			fmt.Fprintf(rc.Err, "unexpand: %s: %v\n", name, err)
			status = 1
			continue
		}
		if err := unexpandStream(r, out, tabs, convertAll, *noUTF8); err != nil {
			fmt.Fprintf(rc.Err, "unexpand: %s: %v\n", name, err)
			status = 1
		}
		if closer != nil {
			closer.Close()
		}
	}
	if err := out.Flush(); err != nil {
		fmt.Fprintf(rc.Err, "unexpand: write error: %v\n", err)
		return 1
	}
	return status
}

func unexpandStream(r io.Reader, w io.Writer, tabs []int, all, noUTF8 bool) error {
	br := bufio.NewReader(r)
	for {
		line, err := br.ReadString('\n')
		if len(line) > 0 {
			var out string
			if noUTF8 {
				out = unexpandBytes(line, tabs, all)
			} else {
				out = unexpandLine(line, tabs, all)
			}
			if _, werr := io.WriteString(w, out); werr != nil {
				return werr
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func unexpandBytes(line string, tabs []int, all bool) string {
	var b strings.Builder
	col := 0
	spaceRun := 0
	leading := true
	flush := func() {
		for spaceRun > 0 {
			next := nextStop(col, tabs)
			if spaceRun >= next-col && next > col {
				b.WriteByte('\t')
				spaceRun -= next - col
				col = next
			} else {
				b.WriteByte(' ')
				spaceRun--
				col++
			}
		}
	}
	for i := 0; i < len(line); i++ {
		c := line[i]
		if c == ' ' && (all || leading) {
			spaceRun++
			continue
		}
		flush()
		switch c {
		case '\n':
			b.WriteByte(c)
			col = 0
			leading = true
		case '\t':
			b.WriteByte(c)
			col = nextStop(col, tabs)
		default:
			b.WriteByte(c)
			col++
			leading = false
		}
	}
	flush()
	return b.String()
}

func unexpandLine(line string, tabs []int, all bool) string {
	var b strings.Builder
	col := 0
	spaceRun := 0
	leading := true
	flush := func() {
		for spaceRun > 0 {
			next := nextStop(col, tabs)
			if spaceRun >= next-col && next > col {
				b.WriteByte('\t')
				spaceRun -= next - col
				col = next
			} else {
				b.WriteByte(' ')
				spaceRun--
				col++
			}
		}
	}
	for _, r := range line {
		if r == ' ' && (all || leading) {
			spaceRun++
			continue
		}
		flush()
		switch r {
		case '\n':
			b.WriteRune(r)
			col = 0
			leading = true
		case '\t':
			b.WriteRune(r)
			col = nextStop(col, tabs)
		default:
			b.WriteRune(r)
			col++
			leading = false
		}
	}
	flush()
	return b.String()
}

func parseTabStops(s string) ([]int, error) {
	if s == "" {
		return nil, fmt.Errorf("empty")
	}
	parts := strings.Split(s, ",")
	out := make([]int, 0, len(parts))
	prev := 0
	for _, p := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil || n <= 0 || n <= prev {
			return nil, fmt.Errorf("invalid")
		}
		out = append(out, n)
		prev = n
	}
	return out, nil
}

func nextStop(col int, stops []int) int {
	if len(stops) == 1 {
		n := stops[0]
		return ((col / n) + 1) * n
	}
	for _, stop := range stops {
		if stop > col {
			return stop
		}
	}
	return col + 1
}

func openInput(rc *tool.RunContext, name string) (io.Reader, io.Closer, error) {
	if name == "-" {
		if rc.In == nil {
			return strings.NewReader(""), nil, nil
		}
		return rc.In, nil, nil
	}
	f, err := os.Open(rc.Path(name))
	if err != nil {
		return nil, nil, err
	}
	return f, f, nil
}
