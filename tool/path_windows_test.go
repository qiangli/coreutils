//go:build windows

package tool

import "testing"

// TestMSYSDriveMapping covers the new MSYS/Git-Bash drive convention (/c/ -> C:\)
// honored by both the rc.Path entry (normalizePath) and the localFS layer
// (toOSPath). The legacy drive-less "/foo -> SystemDrive" and the round-trip are
// covered by TestNormalizePath/TestToOSPath/TestFromOSPath in tool_test.go.
func TestMSYSDriveMapping(t *testing.T) {
	cases := []struct{ in, want string }{
		{"/c/Users/Lern", `C:\Users\Lern`},
		{"/c", `C:\`},
		{"/d/foo/bar", `D:\foo\bar`},
		{"/C/Up", `C:\Up`}, // uppercase drive letter
	}
	for _, c := range cases {
		if got := normalizePath(c.in); got != c.want {
			t.Errorf("normalizePath(%q) = %q, want %q", c.in, got, c.want)
		}
		if got := toOSPath(c.in); got != c.want {
			t.Errorf("toOSPath(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	// A drive-less path like /foo/bar is NOT a drive reference; normalizePath
	// leaves it drive-relative (slash-converted only).
	if got := normalizePath("/foo/bar"); got != `\foo\bar` {
		t.Errorf("normalizePath(/foo/bar) = %q, want %q", got, `\foo\bar`)
	}
}
