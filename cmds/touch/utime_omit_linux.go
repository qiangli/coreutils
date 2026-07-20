//go:build linux

package touchcmd

// utimeOmit is Linux's UTIME_OMIT and utimeNow its UTIME_NOW: the utimensat
// tv_nsec sentinels meaning "leave this timestamp unchanged" and "set it to the
// current time". Darwin and Linux disagree on the magic numbers, so they are
// defined per platform.
const (
	utimeOmit = (1 << 30) - 2
	utimeNow  = (1 << 30) - 1
)
