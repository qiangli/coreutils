// Package ptxcmd implements a small ptx(1) subset: generate a
// deterministic permuted index from input words.
package ptxcmd

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"
	"unicode"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "ptx",
	Synopsis: "Produce a permuted index of file contents.",
	Usage:    "ptx [OPTION]... [FILE]...",
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	ignoreCase := fs.BoolP("ignore-case", "f", false, "fold lower case to upper case for sorting")
	onlyFile := fs.StringP("only-file", "o", "", "read only these words")
	ignoreFile := fs.StringP("ignore-file", "i", "", "ignore these words")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	only, err := readWordSet(rc, *onlyFile)
	if err != nil {
		fmt.Fprintf(rc.Err, "ptx: cannot read '%s': %v\n", *onlyFile, tool.SysErr(err))
		return 1
	}
	ignore, err := readWordSet(rc, *ignoreFile)
	if err != nil {
		fmt.Fprintf(rc.Err, "ptx: cannot read '%s': %v\n", *ignoreFile, tool.SysErr(err))
		return 1
	}
	if len(operands) == 0 {
		operands = []string{"-"}
	}
	status := 0
	var entries []indexEntry
	for _, name := range operands {
		fileEntries, code := readEntries(rc, name, only, ignore, *ignoreCase)
		if code != 0 {
			status = 1
		}
		entries = append(entries, fileEntries...)
	}
	sort.SliceStable(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if a.sortKey != b.sortKey {
			return a.sortKey < b.sortKey
		}
		if a.right != b.right {
			return a.right < b.right
		}
		return a.left < b.left
	})
	for _, entry := range entries {
		fmt.Fprintf(rc.Out, "%-32s\t%s\t%s\n", entry.left, entry.word, entry.right)
	}
	return status
}

func readWordSet(rc *tool.RunContext, name string) (map[string]bool, error) {
	if name == "" {
		return nil, nil
	}
	f, err := os.Open(rc.Path(name))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	set := make(map[string]bool)
	sc := bufio.NewScanner(f)
	sc.Split(bufio.ScanWords)
	for sc.Scan() {
		set[strings.ToLower(sc.Text())] = true
	}
	return set, sc.Err()
}

type indexEntry struct {
	left    string
	word    string
	right   string
	sortKey string
}

func readEntries(rc *tool.RunContext, name string, only, ignore map[string]bool, ignoreCase bool) ([]indexEntry, int) {
	var sc *bufio.Scanner
	var f *os.File
	if name == "-" {
		sc = bufio.NewScanner(rc.In)
	} else {
		var err error
		f, err = os.Open(rc.Path(name))
		if err != nil {
			fmt.Fprintf(rc.Err, "ptx: cannot open '%s' for reading: %v\n", name, tool.SysErr(err))
			return nil, 1
		}
		defer f.Close()
		sc = bufio.NewScanner(f)
	}
	var entries []indexEntry
	for sc.Scan() {
		entries = append(entries, lineEntries(sc.Text(), only, ignore, ignoreCase)...)
	}
	if err := sc.Err(); err != nil {
		fmt.Fprintf(rc.Err, "ptx: read error: %v\n", tool.SysErr(err))
		return entries, 1
	}
	return entries, 0
}

func lineEntries(line string, only, ignore map[string]bool, ignoreCase bool) []indexEntry {
	words := strings.Fields(line)
	entries := make([]indexEntry, 0, len(words))
	for i, word := range words {
		key := strings.ToLower(strings.TrimFunc(word, func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsDigit(r)
		}))
		if key == "" {
			continue
		}
		if only != nil && !only[key] {
			continue
		}
		if ignore != nil && ignore[key] {
			continue
		}
		sortKey := strings.TrimFunc(word, func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsDigit(r)
		})
		if ignoreCase {
			sortKey = strings.ToLower(sortKey)
		}
		left := strings.Join(words[:i], " ")
		right := strings.Join(words[i+1:], " ")
		entries = append(entries, indexEntry{left: left, word: word, right: right, sortKey: sortKey})
	}
	return entries
}
