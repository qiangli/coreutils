//go:build !unix

package nicecmd

import "fmt"

func currentPriority() int { return 0 }

func setPriority(int, int) error {
	return fmt.Errorf("priority adjustment is not supported on this platform")
}
