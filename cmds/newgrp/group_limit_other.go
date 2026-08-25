//go:build !linux && !darwin && !dragonfly && !freebsd && !netbsd && !openbsd

package newgrpcmd

func maximumSupplementaryGroups() int { return 0 }
