//go:build windows

package tool

func posixErrorText(err error) (string, bool) { return posixErrorTextMode(err, true) }
