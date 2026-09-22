package localecmd

import (
	"os"
	"sort"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

// The enumeration roots are package variables so tests can point them at a
// fixture tree. Without that, `locale -a`'s output would depend on which
// locales the developer's machine happens to have installed, and the test
// would assert nothing repeatable.
var (
	// charmapDirs holds charmap definitions, commonly gzipped.
	charmapDirs = []string{
		"/usr/share/i18n/charmaps",
	}
)

// availableLocales lists the locale names this host can offer.
//
// C and POSIX are always present because they are required to exist by the
// standard and are not files anyone installs — omitting them when no locale
// directory exists would report a host that cannot run a conforming program,
// which is the opposite of the truth.
func availableLocales(rc *tool.RunContext) []string {
	return availableLocalesFromProvider(defaultHostLocaleProvider(rc))
}

func availableLocalesFromProvider(provider *hostLocaleProvider) []string {
	// Do not advertise arbitrary host locale-directory names that localeData
	// cannot subsequently serve. The German fixture data is carried in the
	// built-in database for both the single-byte and UTF-8 spellings, so those
	// names are available on every host too.
	available := []string{"C", "POSIX", "de_DE.ISO-8859-1", "de_DE.UTF-8"}
	if provider == nil {
		return available
	}
	seen := make(map[string]bool, len(available))
	for _, name := range available {
		seen[name] = true
	}
	for _, name := range provider.names() {
		if name == "" || seen[name] || !provider.serves(name) {
			continue
		}
		seen[name] = true
		available = append(available, name)
	}
	// Keep the carried names in their established order and make provisioned
	// host names deterministic even if the host service enumerates differently.
	sort.Strings(available[4:])
	return available
}

// availableCharmaps lists the charmap names this host can offer. Charmap files
// are conventionally stored gzipped, and the compression is not part of the
// name.
func availableCharmaps() []string {
	seen := map[string]bool{
		posixCharmap:    true,
		iso88591Charmap: true,
	}
	for _, dir := range charmapDirs {
		for _, e := range readDir(dir) {
			name := e.Name()
			if strings.HasPrefix(name, ".") {
				continue
			}
			seen[strings.TrimSuffix(name, ".gz")] = true
		}
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
