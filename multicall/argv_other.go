//go:build !windows

package multicall

// decodeSurrogateArgs is the identity off Windows: argv is bytes there.
func decodeSurrogateArgs(args []string) []string { return args }
