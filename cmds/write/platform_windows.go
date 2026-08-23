//go:build windows

package writecmd

import "errors"

// Windows has neither of the two things write(1) is built on: a login
// accounting database of the utmp family, and a terminal device whose
// group-write permission bit decides whether messages are accepted.
//
// Refusing is the only honest option. Go synthesizes os.FileMode on Windows
// from the read-only attribute alone, so a permission check there would report
// "messages allowed" for every path — a write that appears to succeed and
// reaches nobody, which is exactly the failure this tool must not produce.
const defaultUtmpPath = ""

var activeUtmpLayout = utmpLayout{}

const platformSupported = false

var errPlatform = errors.New("terminal messaging is not a Windows concept: there is no login accounting database and no terminal message-permission bit")
