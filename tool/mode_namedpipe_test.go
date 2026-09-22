package tool

import (
	"io/fs"
	"testing"
)

func TestNamedPipeName(t *testing.T) {
	for _, tc := range []struct {
		path string
		name string
		ok   bool
	}{
		{`//./pipe/sh-np-a1`, "sh-np-a1", true},
		{`\\.\pipe\sh-np-a1`, "sh-np-a1", true},
		{`//./pipe/`, "", false},
		{`//./pipe/sh-np-a1/child`, "", false},
		{`C:\\tmp\\sh-np-a1`, "", false},
	} {
		got, ok := namedPipeName(tc.path)
		if got != tc.name || ok != tc.ok {
			t.Errorf("namedPipeName(%q) = %q, %v; want %q, %v", tc.path, got, ok, tc.name, tc.ok)
		}
	}
}

func TestNamedPipeInfo(t *testing.T) {
	fi := namedPipeInfo{name: "sh-np-a1"}
	if fi.Name() != "sh-np-a1" || fi.Size() != 0 || fi.IsDir() || !fi.ModTime().IsZero() {
		t.Errorf("namedPipeInfo = name=%q size=%d dir=%v time=%v", fi.Name(), fi.Size(), fi.IsDir(), fi.ModTime())
	}
	if want := fs.ModeNamedPipe | 0o600; fi.Mode() != want {
		t.Errorf("namedPipeInfo.Mode() = %v, want %v", fi.Mode(), want)
	}
}
