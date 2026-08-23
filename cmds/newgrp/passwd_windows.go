//go:build windows

package newgrpcmd

// Windows has no password file and no login-shell field. Returning "" lets the
// shared lookup fall through to $SHELL, which is all the command can honestly
// offer here — and the command refuses on Windows anyway (see spawn_windows.go).
func passwdShell(string) string { return "" }
