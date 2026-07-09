package gosed

// CompileSimpleSubstitution returns the same regexp and replacement template
// used by the full s/// command implementation.
func CompileSimpleSubstitution(pattern, replacement string) (sedRegexp, []byte, error) {
	rx, err := compileRE(pattern, "")
	if err != nil {
		return nil, nil, err
	}
	return rx, []byte(translateReplacement(replacement)), nil
}
