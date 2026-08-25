//go:build !unix

package batchcmd

func processUmask() (uint32, bool) { return 0, false }
