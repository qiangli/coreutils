// Package ptxcmd implements a small ptx(1) subset: generate a
// deterministic permuted index from input words.
package ptxcmd

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
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
	autoReference := fs.BoolP("auto-reference", "A", false, "output automatically generated references")
	traditional := fs.BoolP("traditional", "G", false, "behave more like System V ptx")
	typeset := fs.BoolP("typeset-mode", "t", false, "generate a simple typesetting-oriented output")
	rightRefs := fs.BoolP("right-side-refs", "R", false, "put references at the right side")
	breakFile := fs.StringP("break-file", "b", "", "use characters in FILE as word break characters")
	sentenceRegexp := fs.StringP("sentence-regexp", "S", "", "use REGEXP to recognize sentence boundaries")
	ignoreCase := fs.BoolP("ignore-case", "f", false, "fold lower case to upper case for sorting")
	gapSize := fs.IntP("gap-size", "g", 0, "gap size in columns between output fields")
	onlyFile := fs.StringP("only-file", "o", "", "read only these words")
	ignoreFile := fs.StringP("ignore-file", "i", "", "ignore these words")
	references := fs.BoolP("references", "r", false, "first field of each line is a reference")
	width := fs.IntP("width", "w", 0, "output width in columns")
	wordRegexp := fs.StringP("word-regexp", "W", "", "use REGEXP to match each keyword")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	var wordRE *regexp.Regexp
	if *wordRegexp != "" {
		var err error
		wordRE, err = regexp.Compile(*wordRegexp)
		if err != nil {
			return tool.UsageError(rc, cmd, "invalid word regexp '%s'", *wordRegexp)
		}
	}
	var sentenceRE *regexp.Regexp
	if *sentenceRegexp != "" {
		var err error
		sentenceRE, err = regexp.Compile(*sentenceRegexp)
		if err != nil {
			return tool.UsageError(rc, cmd, "invalid sentence regexp '%s'", *sentenceRegexp)
		}
	}
	breakChars, err := readBreakChars(rc, *breakFile)
	if err != nil {
		fmt.Fprintf(rc.Err, "ptx: cannot read '%s': %v\n", *breakFile, tool.SysErr(err))
		return 1
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
	opts := ptxOptions{
		ignoreCase:    *ignoreCase,
		autoReference: *autoReference,
		references:    *references,
		traditional:   *traditional,
		typeset:       *typeset,
		rightRefs:     *rightRefs,
		width:         *width,
		gapSize:       *gapSize,
		wordRE:        wordRE,
		sentenceRE:    sentenceRE,
		breakChars:    breakChars,
	}
	for _, name := range operands {
		fileEntries, code := readEntries(rc, name, only, ignore, opts)
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
		fmt.Fprintln(rc.Out, formatEntry(entry, opts))
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

func readBreakChars(rc *tool.RunContext, name string) (string, error) {
	if name == "" {
		return "", nil
	}
	data, err := os.ReadFile(rc.Path(name))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

type indexEntry struct {
	reference string
	left      string
	word      string
	right     string
	sortKey   string
}

type ptxOptions struct {
	ignoreCase    bool
	autoReference bool
	references    bool
	traditional   bool
	typeset       bool
	rightRefs     bool
	width         int
	gapSize       int
	wordRE        *regexp.Regexp
	sentenceRE    *regexp.Regexp
	breakChars    string
}

func readEntries(rc *tool.RunContext, name string, only, ignore map[string]bool, opts ptxOptions) ([]indexEntry, int) {
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
	lineNo := 0
	for sc.Scan() {
		lineNo++
		entries = append(entries, lineEntries(sc.Text(), lineNo, only, ignore, opts)...)
	}
	if err := sc.Err(); err != nil {
		fmt.Fprintf(rc.Err, "ptx: read error: %v\n", tool.SysErr(err))
		return entries, 1
	}
	return entries, 0
}

func lineEntries(line string, lineNo int, only, ignore map[string]bool, opts ptxOptions) []indexEntry {
	ref := ""
	segments := []string{line}
	if opts.sentenceRE != nil {
		segments = splitSentences(line, opts.sentenceRE)
	}
	var entries []indexEntry
	for _, segment := range segments {
		entries = append(entries, segmentEntries(segment, lineNo, ref, only, ignore, opts)...)
	}
	return entries
}

func segmentEntries(line string, lineNo int, ref string, only, ignore map[string]bool, opts ptxOptions) []indexEntry {
	words := splitWords(line, opts.breakChars)
	if opts.references && len(words) > 0 {
		ref = words[0]
		words = words[1:]
	} else if opts.autoReference {
		ref = ":" + strconv.Itoa(lineNo)
	}
	entries := make([]indexEntry, 0, len(words))
	for _, occ := range wordOccurrences(words, opts.wordRE) {
		key := strings.ToLower(cleanWord(occ.word))
		if key == "" {
			continue
		}
		if only != nil && !only[key] {
			continue
		}
		if ignore != nil && ignore[key] {
			continue
		}
		sortKey := cleanWord(occ.word)
		if opts.ignoreCase {
			sortKey = strings.ToLower(sortKey)
		}
		entries = append(entries, indexEntry{reference: ref, left: occ.left, word: occ.word, right: occ.right, sortKey: sortKey})
	}
	return entries
}

func splitSentences(line string, re *regexp.Regexp) []string {
	matches := re.FindAllStringIndex(line, -1)
	if len(matches) == 0 {
		return []string{line}
	}
	var out []string
	start := 0
	for _, m := range matches {
		end := m[1]
		if end > start {
			out = append(out, strings.TrimSpace(line[start:end]))
		}
		start = end
	}
	if start < len(line) {
		out = append(out, strings.TrimSpace(line[start:]))
	}
	return out
}

type wordOccurrence struct {
	left  string
	word  string
	right string
}

func splitWords(line, breakChars string) []string {
	if breakChars == "" {
		return strings.Fields(line)
	}
	return strings.FieldsFunc(line, func(r rune) bool {
		return unicode.IsSpace(r) || strings.ContainsRune(breakChars, r)
	})
}

func wordOccurrences(words []string, wordRE *regexp.Regexp) []wordOccurrence {
	if wordRE == nil {
		out := make([]wordOccurrence, 0, len(words))
		for i, word := range words {
			out = append(out, wordOccurrence{
				left:  strings.Join(words[:i], " "),
				word:  word,
				right: strings.Join(words[i+1:], " "),
			})
		}
		return out
	}
	line := strings.Join(words, " ")
	matches := wordRE.FindAllStringIndex(line, -1)
	out := make([]wordOccurrence, 0, len(matches))
	for _, m := range matches {
		out = append(out, wordOccurrence{
			left:  strings.TrimSpace(line[:m[0]]),
			word:  line[m[0]:m[1]],
			right: strings.TrimSpace(line[m[1]:]),
		})
	}
	return out
}

func cleanWord(word string) string {
	return strings.TrimFunc(word, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

func formatEntry(entry indexEntry, opts ptxOptions) string {
	left := entry.left
	right := entry.right
	if opts.width > 0 {
		leftBudget := opts.width / 2
		rightBudget := opts.width - leftBudget
		if len(left) > leftBudget {
			left = left[len(left)-leftBudget:]
		}
		if len(right) > rightBudget {
			right = right[:rightBudget]
		}
	}
	if opts.typeset {
		body := fmt.Sprintf(".xx %q %q %q", entry.word, left, right)
		if entry.reference != "" {
			body += fmt.Sprintf(" %q", entry.reference)
		}
		return body
	}
	if opts.traditional {
		body := strings.Join(nonEmpty(left, entry.word, right), " ")
		if entry.reference != "" {
			if opts.rightRefs {
				return body + " " + entry.reference
			}
			return entry.reference + " " + body
		}
		return body
	}
	if opts.gapSize > 0 {
		gap := strings.Repeat(" ", opts.gapSize)
		body := left + gap + entry.word + gap + right
		if entry.reference != "" {
			if opts.rightRefs {
				return body + gap + entry.reference
			}
			return entry.reference + gap + body
		}
		return body
	}
	body := fmt.Sprintf("%-32s\t%s\t%s", left, entry.word, right)
	if entry.reference != "" {
		if opts.rightRefs {
			return fmt.Sprintf("%s\t%s", body, entry.reference)
		}
		return fmt.Sprintf("%-8s\t%s", entry.reference, body)
	}
	return body
}

func nonEmpty(parts ...string) []string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
