//go:build linux && (amd64 || 386 || arm)

package writecmd

var activeUtmpLayout = layoutLinuxUtmpCompat32

const platformSupported = true
