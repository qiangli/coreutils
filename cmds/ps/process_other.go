//go:build !linux

package pscmd

func enrich(*process)    {}
func currentUID() int    { return -1 }
func currentTTY() string { return "" }
