//go:build !unix

package findcmd

// raiseSelfSignal is a no-op where POSIX signals do not exist (Windows);
// the signal-mapping test lives in exec_signal_unix_test.go and does not
// run here. Defined so the untagged TestMain compiles on every target.
func raiseSelfSignal(int) {}
