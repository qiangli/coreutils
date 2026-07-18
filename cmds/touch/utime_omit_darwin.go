//go:build darwin

package touchcmd

// utimeOmit is Darwin's UTIME_OMIT (sys/stat.h), the utimensat tv_nsec value
// meaning "leave this timestamp unchanged". Darwin and Linux disagree on the
// magic number, so it is defined per platform.
const utimeOmit = -2
