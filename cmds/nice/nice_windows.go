//go:build windows

package nicecmd

func currentNice() int    { return 0 }
func setNice(n int) error { return nil }
