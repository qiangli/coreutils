//go:build windows

package sttycmd

func applyMode(fd int, mode string) error { return nil }

func applyValue(fd int, name string, value uint8) error { return nil }
