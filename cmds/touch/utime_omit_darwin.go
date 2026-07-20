//go:build darwin

package touchcmd

// utimeOmit is Darwin's UTIME_OMIT and utimeNow its UTIME_NOW (sys/stat.h): the
// utimensat tv_nsec sentinels meaning "leave this timestamp unchanged" and "set
// it to the current time". Darwin and Linux disagree on the magic numbers, so
// they are defined per platform.
const (
	utimeOmit = -2
	utimeNow  = -1
)
