//go:build windows

package tool

func commandShellFallback(_ string, _ []string, _ error) (string, []string, bool) {
	return "", nil, false
}
