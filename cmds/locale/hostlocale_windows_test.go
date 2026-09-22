//go:build windows

package localecmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

// TestHostLocaleProviderRunnerProbe is opt-in because it needs a native POSIX
// locale executable on the Windows host. It identifies the exact provider
// predicate that rejects a corpus locale without running the full fixtures.
func TestHostLocaleProviderRunnerProbe(t *testing.T) {
	if os.Getenv("BASHY_LOCALE_DIAG") != "1" {
		t.Skip("set BASHY_LOCALE_DIAG=1 with BASHY_HOST_LOCALE on a Windows host")
	}
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	rc := &tool.RunContext{Dir: dir, Env: os.Environ()}
	path := rc.Getenv(hostLocalePathEnv)
	t.Logf("host path=%q absolute=%v candidate names=%q", path, filepath.IsAbs(path), rc.Getenv(hostLocaleNamesEnv))
	for _, entry := range rc.Env {
		key, _, ok := strings.Cut(entry, "=")
		if ok && strings.EqualFold(key, "LC_ALL") {
			t.Logf("inherited locale selector=%q", entry)
		}
	}
	provider := defaultHostLocaleProvider(rc)
	if provider == nil {
		t.Fatal("defaultHostLocaleProvider returned nil")
	}
	for _, name := range []string{"en_US.UTF-8", "zh_TW.big5", "ja_JP.SJIS", "fr_FR.ISO8859-1", "de_DE.UTF-8", "ru_RU.CP1251", "zh_HK.big5hkscs"} {
		if name == "en_US.UTF-8" {
			stdout, stderr, err := provider.run([]string{"LC_ALL=" + name}, nil)
			t.Logf("raw %s locale: stdout=%q stderr=%q err=%v", name, stdout, stderr, err)
			stdout, stderr, err = provider.run([]string{"LC_ALL=" + name}, []string{"-k", "LC_CTYPE"})
			t.Logf("raw %s LC_CTYPE: stdout=%q stderr=%q err=%v", name, stdout, stderr, err)
		}
		selected := provider.selected(name)
		charmap := provider.matchesRequestedCharmap(name)
		t.Logf("%s: selected=%v matching_charmap=%v", name, selected, charmap)
		for _, category := range categories {
			_, err := provider.query(name, category)
			t.Logf("%s %s: %v", name, category, err)
		}
		t.Logf("%s: serves=%v", name, provider.serves(name))
	}
}
