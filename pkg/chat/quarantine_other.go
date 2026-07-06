//go:build !darwin

package chat

// stripQuarantine is a macOS-only concern (Gatekeeper com.apple.quarantine);
// a no-op everywhere else.
func stripQuarantine(string) {}
