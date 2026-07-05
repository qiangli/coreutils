package gosed

import "regexp"

// CompileSimpleSubstitution returns the same regexp and replacement template
// used by the full s/// command implementation.
func CompileSimpleSubstitution(pattern, replacement string) (*regexp.Regexp, []byte, error) {
	rx, err := compileRE(pattern, "")
	if err != nil {
		return nil, nil, err
	}
	return rx, []byte(translateReplacement(replacement)), nil
}
