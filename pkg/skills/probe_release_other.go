//go:build !darwin

package skills

func darwinProductVersion() (string, error) { return "", ErrNotApplicable }
