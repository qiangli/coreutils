//go:build linux

package touchcmd

// utimeOmit is Linux's UTIME_OMIT, the utimensat tv_nsec value meaning
// "leave this timestamp unchanged". Darwin and Linux disagree on the magic
// number, so it is defined per platform.
const utimeOmit = (1 << 30) - 2
