package loggercmd

import "os/user"

// currentUserName is the last resort for the default tag, used only when
// neither LOGNAME nor USER is set in the invocation environment. It is a
// separate seam so a test can make the default deterministic without touching
// the process's real identity.
var currentUserName = func() string {
	u, err := user.Current()
	if err != nil {
		return ""
	}
	return u.Username
}
