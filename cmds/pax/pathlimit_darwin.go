//go:build darwin

package paxcmd

// destinationPathMax is the {PATH_MAX} of the destination hierarchy,
// counting the terminating NUL. Darwin's limit is 1024 bytes; the generic
// interchange ceiling of 4096 would admit member pathnames the filesystem
// then rejects with ENAMETOOLONG mid-extraction.
const destinationPathMax = 1024
