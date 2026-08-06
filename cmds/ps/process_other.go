//go:build !linux

package pscmd

func enrich(*process) {}
func currentUID() int { return -1 }
