//go:build !linux && !darwin

package pathchkcmd

import "fmt"

func filesystemLimits(string) (int, int, error) {
	return 0, 0, fmt.Errorf("filesystem PATH_MAX and NAME_MAX queries are unsupported on this platform")
}
