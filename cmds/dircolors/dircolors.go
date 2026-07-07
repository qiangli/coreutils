// Package dircolorscmd implements a useful GNU-compatible subset of
// dircolors(1): emit Bourne-shell or C-shell setup code for LS_COLORS,
// print the built-in database, and parse simple DIR/FILE/extension
// color database lines.
package dircolorscmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
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
			fmt.Fprintf(rc.Err, "dircolors: %s: %v\n", operands[0], tool.SysErr(err))
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
	entries, _ := parseDatabase(strings.NewReader(defaultDatabase), "xterm-256color")
	return entries
}

func parseDatabaseFile(rc *tool.RunContext, name string) ([]entry, error) {
	f, err := os.Open(rc.Path(name))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parseDatabase(f, rc.Getenv("TERM"))
}

func parseDatabase(r io.Reader, term string) ([]entry, error) {
	scanner := bufio.NewScanner(r)
	var entries []entry
	var termPatterns []string
	for scanner.Scan() {
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
			continue
		}
		key, value := fields[0], fields[1]
		if strings.EqualFold(key, "TERM") {
			termPatterns = append(termPatterns, value)
			continue
		}
		encoded, ok := encodeKey(key)
		if !ok {
			continue
		}
		entries = append(entries, entry{key: encoded, value: value})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(termPatterns) > 0 && !termMatches(term, termPatterns) {
		return nil, nil
	}
	return entries, nil
}

func encodeKey(key string) (string, bool) {
	if strings.HasPrefix(key, ".") {
		return "*" + key, true
	}
	if strings.HasPrefix(key, "*.") {
		return key, true
	}
	v, ok := typeKeys[strings.ToUpper(key)]
	return v, ok
}

func termMatches(term string, patterns []string) bool {
	if term == "" {
		term = "unknown"
	}
	for _, pattern := range patterns {
		ok, err := filepath.Match(pattern, term)
		if err == nil && ok {
			return true
		}
		if pattern == term {
			return true
		}
	}
	return false
}

func encodeLSColors(entries []entry) string {
	if len(entries) == 0 {
		return ""
	}
	seen := make(map[string]int, len(entries))
	var out []entry
	for _, ent := range entries {
		if i, ok := seen[ent.key]; ok {
			out[i].value = ent.value
			continue
		}
		seen[ent.key] = len(out)
		out = append(out, ent)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].key < out[j].key
	})
	parts := make([]string, 0, len(out))
	for _, ent := range out {
		parts = append(parts, ent.key+"="+ent.value)
	}
	return strings.Join(parts, ":") + ":"
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
