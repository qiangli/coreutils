//go:build !darwin

package paxcmd

// destinationPathMax is the {PATH_MAX} of the destination hierarchy,
// counting the terminating NUL. 4096 is Linux's pathname-copy ABI limit and
// doubles as the conservative ceiling on platforms without a fixed
// compile-time value.
const destinationPathMax = 4096
