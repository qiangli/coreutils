package localecmd

import (
	"os"
	"sort"
	"strings"
)

// The enumeration roots are package variables so tests can point them at a
// fixture tree. Without that, `locale -a`'s output would depend on which
// locales the developer's machine happens to have installed, and the test
// would assert nothing repeatable.
var (
	// localeDirs are the conventional homes of locale definitions (the source
	// forms under /usr/share/i18n) and compiled locales (/usr/lib/locale on
	// glibc systems, /usr/share/locale on the BSDs and macOS).
	localeDirs = []string{
		"/usr/share/i18n/locales",
		"/usr/lib/locale",
		"/usr/share/locale",
	}
	// charmapDirs holds charmap definitions, commonly gzipped.
	charmapDirs = []string{
		"/usr/share/i18n/charmaps",
	}
)

// nonLocaleEntries are files that live alongside locale directories but are not
// locales. locale-archive in particular is a single packed file holding many
// compiled locales; listing it as a locale name would offer the caller a
// setting that cannot be selected.
var nonLocaleEntries = map[string]bool{
	"locale-archive":      true,
	"locale-archive.tmpl": true,
	"locale.alias":        true,
}

// availableLocales lists the locale names this host can offer.
//
// C and POSIX are always present because they are required to exist by the
// standard and are not files anyone installs — omitting them when no locale
// directory exists would report a host that cannot run a conforming program,
// which is the opposite of the truth.
func availableLocales() []string {
	seen := map[string]bool{"C": true, "POSIX": true}
	for _, dir := range localeDirs {
		for _, e := range readDir(dir) {
			name := e.Name()
			if strings.HasPrefix(name, ".") || nonLocaleEntries[name] {
				continue
			}
			seen[name] = true
		}
	}
	return sortedKeys(seen)
}

// availableCharmaps lists the charmap names this host can offer. Charmap files
// are conventionally stored gzipped, and the compression is not part of the
// name.
func availableCharmaps() []string {
	seen := map[string]bool{}
	for _, dir := range charmapDirs {
		for _, e := range readDir(dir) {
			name := e.Name()
			if strings.HasPrefix(name, ".") {
				continue
			}
			seen[strings.TrimSuffix(name, ".gz")] = true
		}
	}
	if len(seen) == 0 {
		// No charmap directory on this host. The POSIX locale's charmap still
		// exists — it is built in, not installed — so reporting an empty list
		// would deny a charmap the implementation demonstrably supports.
		seen[posixCharmap] = true
	}
	return sortedKeys(seen)
}

func readDir(dir string) []os.DirEntry {
	entries, err := os.ReadDir(dir)
	if err != nil {
		// A missing or unreadable directory is normal — most hosts have only
		// one of the three — and contributes nothing rather than failing the
		// whole listing.
		return nil
	}
	return entries
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
