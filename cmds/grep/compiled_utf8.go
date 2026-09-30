package grepcmd

import (
	"regexp"

	"github.com/qiangli/coreutils/pkg/bre"
)

// Compile each pattern independently so alternatives preserve capture numbers.
func compileCompiledUTF8Patterns(patterns []string, fixed, extended, line, fold bool, tables *bre.LocaleByteTables) (grepMatcher, error) {
	var out multiMatcher
	flags := ""
	if fold {
		flags = "(?i)"
	}
	for _, pattern := range patterns {
		syntax := extended
		if fixed {
			pattern = regexp.QuoteMeta(pattern)
			syntax = true
		}
		if line {
			if syntax {
				pattern = "^(?:" + pattern + ")$"
			} else {
				pattern = "^" + pattern + "$"
			}
		}
		re, err := bre.CompileCUTF8WithFlags(pattern, flags, syntax, tables)
		if err != nil {
			return nil, err
		}
		re.Longest()
		out = append(out, re)
	}
	return out, nil
}
