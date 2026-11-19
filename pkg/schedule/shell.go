package schedule

// shellPath is the default POSIX shell reference. Certification builds may
// bind it to their staged Go shell with -ldflags '-X
// github.com/qiangli/coreutils/pkg/schedule.shellPath=/absolute/path/to/sh'.
var shellPath = "/bin/sh"

// DefaultShellPath returns the build's POSIX shell reference. It is shared by
// crontab defaults and the scheduler's saved-umask wrapper.
func DefaultShellPath() string { return shellPath }
