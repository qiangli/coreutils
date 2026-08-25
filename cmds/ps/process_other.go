//go:build !linux

package pscmd

func enrich(*process)                                         {}
func enrichWithReader(*process, func(string) ([]byte, error)) {}
func currentUID() int                                         { return -1 }
func currentTTY() string                                      { return "" }
