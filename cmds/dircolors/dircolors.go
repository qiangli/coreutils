// Package dircolorscmd implements a useful GNU-compatible subset of
// dircolors(1): emit Bourne-shell or C-shell setup code for LS_COLORS,
// print the built-in database, and parse color database files with
// GNU's TERM/COLORTERM gating (entries before any TERM/COLORTERM line
// always apply; entries after them apply once any pattern has matched
// the current terminal). Unrecognized keywords and malformed lines are
// errors, as in GNU — never silently skipped.
//
// Documented deviation: with no FILE operand the built-in database is
// emitted in full, independent of $TERM (deterministic output; the
// built-in TERM patterns are not consulted).
package dircolorscmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "dircolors",
	Synopsis: "Output commands to set the LS_COLORS environment variable.",
	Usage:    "dircolors [OPTION]... [FILE]",
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	bourne := fs.BoolP("bourne-shell", "b", false, "output Bourne shell commands")
	cshell := fs.BoolP("c-shell", "c", false, "output C shell commands")
	printDB := fs.BoolP("print-database", "p", false, "output the built-in color database")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	if *bourne && *cshell {
		return tool.UsageError(rc, cmd, "options --bourne-shell and --c-shell are mutually exclusive")
	}
	if *printDB {
		if *bourne || *cshell {
			return tool.UsageError(rc, cmd, "the options to output the internal database and to select a shell syntax are mutually exclusive")
		}
		if len(operands) > 0 {
			return tool.UsageError(rc, cmd, "extra operand '%s'; file operands cannot be combined with --print-database", operands[0])
		}
		fmt.Fprint(rc.Out, defaultDatabase)
		return 0
	}
	if len(operands) > 1 {
		return tool.UsageError(rc, cmd, "extra operand '%s'", operands[1])
	}

	entries := defaultEntries()
	if len(operands) == 1 {
		parsed, err := parseDatabaseFile(rc, operands[0])
		if err != nil {
			fmt.Fprintf(rc.Err, "dircolors: %v\n", err)
			return 1
		}
		entries = parsed
	}

	colors := encodeLSColors(entries)
	if *cshell {
		fmt.Fprintf(rc.Out, "setenv LS_COLORS %s\n", shellQuote(colors))
		return 0
	}
	// GNU dircolors defaults to Bourne-shell output.
	fmt.Fprintf(rc.Out, "LS_COLORS=%s;\nexport LS_COLORS\n", shellQuote(colors))
	return 0
}

type entry struct {
	key   string
	value string
}

var typeKeys = map[string]string{
	"RESET":                 "rs",
	"NORMAL":                "no",
	"NORM":                  "no",
	"FILE":                  "fi",
	"DIR":                   "di",
	"LINK":                  "ln",
	"SYMLINK":               "ln",
	"ORPHAN":                "or",
	"MISSING":               "mi",
	"FIFO":                  "pi",
	"PIPE":                  "pi",
	"SOCK":                  "so",
	"DOOR":                  "do",
	"BLK":                   "bd",
	"BLOCK":                 "bd",
	"CHR":                   "cd",
	"CHAR":                  "cd",
	"EXEC":                  "ex",
	"SETUID":                "su",
	"SETGID":                "sg",
	"STICKY":                "st",
	"OTHER_WRITABLE":        "ow",
	"OWR":                   "ow",
	"STICKY_OTHER_WRITABLE": "tw",
	"OWT":                   "tw",
	"CAPABILITY":            "ca",
	"MULTIHARDLINK":         "mh",
	"LEFTCODE":              "lc",
	"RIGHTCODE":             "rc",
	"ENDCODE":               "ec",
	"CLEAR_SCREEN":          "cl",
}

const defaultDatabase = `# Configuration file for dircolors, a utility to help you set the
# LS_COLORS environment variable used by GNU ls with --color.
TERM *color*
TERM xterm*
TERM screen*
TERM tmux*
TERM rxvt*
RESET 0
DIR 01;34
LINK 01;36
FIFO 40;33
SOCK 01;35
BLK 40;33;01
CHR 40;33;01
ORPHAN 40;31;01
MISSING 00
EXEC 01;32
SETUID 37;41
SETGID 30;43
STICKY_OTHER_WRITABLE 30;42
OTHER_WRITABLE 34;42
STICKY 37;44
.tar 01;31
.tgz 01;31
.zip 01;31
.gz 01;31
.xz 01;31
.zst 01;31
.bz2 01;31
.deb 01;31
.rpm 01;31
.jar 01;31
.jpg 01;35
.jpeg 01;35
.gif 01;35
.bmp 01;35
.tif 01;35
.tiff 01;35
.png 01;35
.svg 01;35
.webp 01;35
.mov 01;35
.mpg 01;35
.mpeg 01;35
.mkv 01;35
.webm 01;35
.mp4 01;35
.avi 01;35
.flv 01;35
.ogv 01;35
.aac 00;36
.flac 00;36
.m4a 00;36
.mid 00;36
.midi 00;36
.mp3 00;36
.ogg 00;36
.opus 00;36
.wav 00;36
`

func defaultEntries() []entry {
	// The built-in database is emitted independent of $TERM (see the
	// package doc); a terminal matching every built-in pattern keeps
	// the parser's gating satisfied.
	entries, _ := parseDatabase(strings.NewReader(defaultDatabase), "<internal>", "xterm-256color", "")
	return entries
}

func parseDatabaseFile(rc *tool.RunContext, name string) ([]entry, error) {
	f, err := os.Open(rc.Path(name))
	if err != nil {
		return nil, fmt.Errorf("%s: %v", name, tool.SysErr(err))
	}
	defer f.Close()
	return parseDatabase(f, name, rc.Getenv("TERM"), rc.Getenv("COLORTERM"))
}

// parse states, per GNU semantics: entries before any TERM/COLORTERM
// line are global; after such lines, entries apply only once some
// pattern has matched — and a later mismatched TERM cannot cancel a
// match already seen.
type parseState int

const (
	stateGlobal parseState = iota
	statePass
	stateMatched
	stateContinue
)

func parseDatabase(r io.Reader, filename, term, colorterm string) ([]entry, error) {
	scanner := bufio.NewScanner(r)
	var entries []entry
	state := stateGlobal
	sawColortermMatch := false
	lineno := 0
	for scanner.Scan() {
		lineno++
		line := scanner.Text()
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return nil, fmt.Errorf("%s:%d: invalid line; missing second token", filename, lineno)
		}
		key, value := fields[0], fields[1]
		switch strings.ToUpper(key) {
		case "TERM":
			if patternMatches(value, term) {
				state = stateMatched
			} else if state == stateGlobal {
				state = statePass
			}
			continue
		case "COLORTERM":
			// COLORTERM ?* matches any non-empty COLORTERM.
			matched := false
			if value == "?*" {
				matched = colorterm != ""
			} else {
				matched = patternMatches(value, colorterm)
			}
			if matched {
				state = stateMatched
				sawColortermMatch = true
			} else if !sawColortermMatch && state == stateGlobal {
				state = statePass
			}
			continue
		case "OPTIONS", "COLOR", "EIGHTBIT":
			// Slackware-only keywords; GNU ignores them.
			continue
		}
		if state == stateMatched {
			state = stateContinue
		}
		if state == statePass {
			continue
		}
		encoded, ok := encodeKey(key)
		if !ok {
			return nil, fmt.Errorf("%s:%d: unrecognized keyword %s", filename, lineno, key)
		}
		entries = append(entries, entry{key: encoded, value: value})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("%s: %v", filename, err)
	}
	return entries, nil
}

func encodeKey(key string) (string, bool) {
	if strings.HasPrefix(key, ".") {
		return "*" + key, true
	}
	if strings.HasPrefix(key, "*") {
		return key, true
	}
	v, ok := typeKeys[strings.ToUpper(key)]
	return v, ok
}

func patternMatches(pattern, s string) bool {
	if s == "" {
		s = "none"
	}
	ok, err := filepath.Match(pattern, s)
	if err == nil && ok {
		return true
	}
	return pattern == s
}

// encodeLSColors joins the entries in database order, without
// deduplication — GNU emits them as parsed; consumers use the last
// match.
func encodeLSColors(entries []entry) string {
	if len(entries) == 0 {
		return ""
	}
	parts := make([]string, 0, len(entries))
	for _, ent := range entries {
		parts = append(parts, ent.key+"="+ent.value)
	}
	return strings.Join(parts, ":") + ":"
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
