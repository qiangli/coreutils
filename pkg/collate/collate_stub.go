// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build !(linux && (amd64 || arm64))

package collate

// Provider is the non-glibc placeholder. It exists so the API type is present on
// every platform; no method on it can succeed because no provider is built here.
type Provider struct{}

// Open validates the locale name exactly as the real provider does — so callers
// on any platform get the same ErrUnsupportedLocale for an unaccepted name — and
// then reports ErrUnsupportedPlatform, because no glibc provider is built off
// linux/amd64 and linux/arm64.
func Open(name string) (*Provider, error) {
	if _, ok := normalizeLocale(name); !ok {
		return nil, ErrUnsupportedLocale
	}
	return nil, ErrUnsupportedPlatform
}

// Compare always fails on the stub: a Provider is never successfully created
// here, so a non-nil receiver cannot exist through the public API.
func (p *Provider) Compare(a, b string) (int, error) { return 0, ErrUnsupportedPlatform }

// Close is a no-op on the stub.
func (p *Provider) Close() error { return nil }
