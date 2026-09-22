package localecmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
)

type hostLocaleCache struct {
	HostPath   string   `json:"host_path"`
	Candidates []string `json:"candidates"`
	HostNames  []string `json:"host_names"`
	Available  []string `json:"available"`
}

// A Bashy fixture root is private to one suite run. Its cache is written only
// after exhaustive host verification and is reused by later locale processes.
// Refresh the host name list on every call so an added or removed host locale
// invalidates the snapshot; an ordinary coreutils invocation has no cache path.
func availableLocalesCached(provider *hostLocaleProvider, cachePath, hostPath string) []string {
	if cachePath == "" || !filepath.IsAbs(cachePath) {
		return availableLocalesFromProvider(provider)
	}
	names := provider.names()
	key := hostLocaleCache{HostPath: hostPath, Candidates: provider.candidates, HostNames: names}
	if data, err := os.ReadFile(cachePath); err == nil && len(data) <= 1<<20 {
		var cached hostLocaleCache
		if json.Unmarshal(data, &cached) == nil && cached.HostPath == key.HostPath &&
			slices.Equal(cached.Candidates, key.Candidates) && slices.Equal(cached.HostNames, key.HostNames) &&
			len(cached.Available) >= 4 {
			return cached.Available
		}
	}
	available := availableLocalesFromHostNames(provider, names)
	key.Available = available
	writeHostLocaleCache(cachePath, key)
	return available
}

func writeHostLocaleCache(path string, cache hostLocaleCache) {
	data, err := json.Marshal(cache)
	if err != nil {
		return
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".host-locales-*")
	if err != nil {
		return
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return
	}
	if err = f.Close(); err != nil {
		return
	}
	// Windows Rename does not replace an existing destination. A reader that
	// catches the brief gap simply recomputes from the host.
	_ = os.Remove(path)
	_ = os.Rename(f.Name(), path)
}
