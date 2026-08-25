//go:build linux && !amd64 && !386 && !arm && !arm64

package writecmd

var activeUtmpLayout = utmpLayout{}

const platformSupported = false
