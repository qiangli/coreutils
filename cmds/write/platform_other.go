//go:build !linux && !darwin && !windows

package writecmd

import (
	"errors"
	"runtime"
)

// Other platforms (the BSDs, Solaris, plan9, js/wasm, …) each have their own
// utmpx variant or none at all. Rather than guess at a layout that has not
// been verified against a real database on that system — and quietly address
// the wrong terminal when the guess is off by four bytes — write refuses and
// says which platform it is refusing on.
const defaultUtmpPath = ""

var activeUtmpLayout = utmpLayout{}

const platformSupported = false

var errPlatform = errors.New("the login accounting database layout for " + runtime.GOOS + " has not been verified; terminal messaging is unsupported on this platform")
