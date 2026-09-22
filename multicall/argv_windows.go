//go:build windows

package multicall

import "mvdan.cc/sh/v3/interp"

// decodeSurrogateArgs turns Cygwin lone-surrogate argv spellings (WTF-8
// in os.Args after Go's UTF-16 decode) back into raw bytes.
func decodeSurrogateArgs(args []string) []string { return interp.DecodeWindowsArgs(args) }
