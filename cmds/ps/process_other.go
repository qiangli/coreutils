//go:build !linux

package pscmd

import "fmt"

func prepareProcessSource() (func(*process) bool, error) {
	return nil, fmt.Errorf("live process inspection is supported only on Linux")
}
func enrich(*process) bool                                         { return false }
func enrichWithReader(*process, func(string) ([]byte, error)) bool { return false }
func currentUID() int                                              { return -1 }
func currentTTY() string                                           { return "" }
