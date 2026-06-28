//go:build windows

package tool

import "testing"

func TestMSYSPathMapping(t *testing.T) {
	to := []struct{ in, want string }{
		{"/c/Users/Lern", `C:\Users\Lern`},
		{"/c", `C:\`},
		{"/d/foo/bar", `D:\foo\bar`},
		{"/C/Up", `C:\Up`}, // uppercase drive letter
		{"relative/path", `relative\path`},
	}
	for _, c := range to {
		if got := toOSPath(c.in); got != c.want {
			t.Errorf("toOSPath(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	from := []struct{ in, want string }{
		{`C:\Users\Lern`, "/c/Users/Lern"},
		{`D:\foo`, "/d/foo"},
		{`C:\`, "/c/"},
	}
	for _, c := range from {
		if got := fromOSPath(c.in); got != c.want {
			t.Errorf("fromOSPath(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
