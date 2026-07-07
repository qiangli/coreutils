//go:build windows

package chrootcmd

import "fmt"

func applyChroot(root string, skipChdir bool) error {
	return fmt.Errorf("chroot is not supported on windows")
}

func applyUserSpec(spec, groups string) error { return nil }
