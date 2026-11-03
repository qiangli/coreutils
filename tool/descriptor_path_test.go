package tool

import "testing"

func TestDescriptorPathResolution(t *testing.T) {
	rc := &RunContext{Dir: "/work", ResolveDescriptorPath: func(path string) (string, bool) {
		if path == "/dev/fd/63" {
			return "/dev/fd/8", true
		}
		return "", false
	}}
	for _, resolve := range []func(string) string{rc.Path, rc.RawPath} {
		if got := resolve("/dev/fd/63"); got != "/dev/fd/8" {
			t.Fatalf("owned descriptor=%q", got)
		}
		ordinary := *rc
		ordinary.ResolveDescriptorPath = nil
		for _, path := range []string{"ordinary", "/dev/fd/20", "/dev/fd/63/", "/absolute"} {
			// Both path APIs preserve these ordinary operands identically.
			if got, want := resolve(path), ordinary.Path(path); got != want {
				t.Fatalf("fallback %q=%q want%q", path, got, want)
			}
		}
	}
}
