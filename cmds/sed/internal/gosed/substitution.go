package gosed

// This file has the functionality for substitution and translation.
// They are the most complicated functions, so I didn't want
// to mix them in with the other instructions in instructions.go.

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// ------------------------------------------------------------------
// -  SUBSTITUTION  -------------------------------------------------
// ------------------------------------------------------------------
type substitute struct {
	pattern     sedRegexp // the pattern to match
	replacement string    // the template for replacements
	which       int       // which pattern to replace
	pflag       bool      // do we print upon replacement?
	gflag       bool      // do we replace every match after 'which'?
	wfile       string    // write a changed pattern space to this file
	writeFile   WriteFileFunc
	null        bool // written as s//repl/: the null RE (see resolveNullRE)
}

func (s *substitute) run(svm *vm) (err error) {
	svm.ip++

	// perform the search
	pattern := resolveNullRE(svm, s.pattern, s.null)
	matches := pattern.FindAllStringSubmatchIndex(svm.pat, -1)

	// filter to the matches we want to replace
	var end int = len(matches)
	if s.which < end {
		if !s.gflag {
			end = s.which + 1
		}
	} else {
		// the matches we want weren't found
		return
	}
	matches = matches[s.which:end]

	// perform the replacement
	svm.pat = subst_replaceAll(svm.pat, pattern, s.replacement, matches)
	svm.modified = true

	// print if requested
	if s.pflag {
		err = cmd_print(svm)
		svm.ip-- // roll back ip from the print command
	}
	if err == nil && s.wfile != "" {
		err = s.writeFile(s.wfile, svm.pat)
	}

	return
}

func subst_replaceAll(src string, pattern sedRegexp, replacement string, indexes [][]int) string {
	var substrings []string
	endpt := 0 // where we left off in the src string
	for _, idx := range indexes {
		exp := string(pattern.ExpandString(nil, replacement, src, idx))
		substrings = append(substrings, src[endpt:idx[0]], exp)
		endpt = idx[1]
	}
	substrings = append(substrings, src[endpt:])

	return strings.Join(substrings, "")
}

func newSubstitution(pattern string, replacement string, mods string, wfile string, last string, writeFile WriteFileFunc) (instruction, error) {
	command := &substitute{wfile: wfile, writeFile: writeFile}
	var numbers []rune
	var flags string // RE2 flag prefix accumulated from i/m modifiers

	for _, char := range mods {
		switch char {
		case 'p':
			command.pflag = true
		case 'g':
			command.gflag = true
		case 'i', 'I': // GNU case-insensitive
			if !strings.ContainsRune(flags, 'i') {
				flags += "i"
			}
		case 'm', 'M': // GNU multi-line (^/$ match at embedded newlines)
			if !strings.ContainsRune(flags, 'm') {
				flags += "m"
			}
		case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
			numbers = append(numbers, char)
		default:
			return nil, fmt.Errorf("Bad regexp modifier <%v>", char)
		}
	}

	// An empty pattern is POSIX's null RE — the last RE used. It resolves
	// dynamically at run time (resolveNullRE); last is the lexically previous
	// RE, compiled as the fallback for a null RE reached before any other RE
	// has run. Modifiers would have to change the RE it stands for, so GNU
	// rejects them here rather than silently recompiling someone else's RE.
	if pattern == "" {
		if flags != "" {
			return nil, fmt.Errorf("cannot specify modifiers on empty regexp")
		}
		if last == "" {
			return nil, fmt.Errorf("no previous regular expression")
		}
		command.null = true
		pattern = last
	}

	prefix := ""
	if flags != "" {
		prefix = "(?" + flags + ")"
	}
	rx, err := compileRE(pattern, prefix) // GNU BRE/ERE → RE2 (was regexp.Compile)
	if err != nil {
		return nil, err
	}
	command.pattern = rx
	command.replacement = translateReplacement(replacement) // GNU \1/& → Go template

	if len(numbers) > 0 {
		command.which, _ = strconv.Atoi(string(numbers))
		if command.which > 0 {
			command.which--
		} else {
			return nil, fmt.Errorf("Bad number %d on substitution", command.which)
		}
	}

	return command.run, nil
}

// ------------------------------------------------------------------
// -  TRANSLATION  --------------------------------------------------
// ------------------------------------------------------------------
func newTranslation(pattern string, replacement string) (instruction, error) {
	rc1 := utf8.RuneCountInString(pattern)
	rc2 := utf8.RuneCountInString(replacement)
	if rc1 != rc2 {
		return nil, fmt.Errorf("Translation 'y' pattern and replacement must be equal length")
	}

	// fill out repls array with alternating patterns and their replacements
	var repls = make([]string, rc1+rc2)
	idx := 0
	for _, ch := range pattern {
		repls[idx] = string(ch)
		idx += 2
	}
	idx = 1
	for _, ch := range replacement {
		repls[idx] = string(ch)
		idx += 2
	}

	stringReplacer := strings.NewReplacer(repls...)

	// now return a custom-made instruction for this translation:
	return func(svm *vm) error {
		svm.pat = stringReplacer.Replace(svm.pat)
		svm.ip++
		return nil
	}, nil
}
