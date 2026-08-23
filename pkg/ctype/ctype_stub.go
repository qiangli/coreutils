// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build !(linux && (amd64 || arm64))

package ctype

// Provider is the non-glibc placeholder. It exists so the API type is
// present on every platform; no method on it can succeed because no
// provider is built here.
type Provider struct{}

// Open validates the locale name exactly as the real provider does — so
// callers on any platform get the same ErrUnsupportedLocale for an
// unaccepted name — and then reports ErrUnsupportedPlatform, because no
// glibc provider is built off linux/amd64 and linux/arm64.
func Open(name string) (*Provider, error) {
	if _, _, ok := normalizeLocale(name); !ok {
		return nil, ErrUnsupportedLocale
	}
	return nil, ErrUnsupportedPlatform
}

// IsAlpha always fails on the stub: a Provider is never successfully
// created here, so a non-nil receiver cannot exist through the public API.
func (p *Provider) IsAlpha(c byte) (bool, error) { return false, ErrUnsupportedPlatform }

// IsAlnum always fails on the stub.
func (p *Provider) IsAlnum(c byte) (bool, error) { return false, ErrUnsupportedPlatform }

// IsBlank always fails on the stub.
func (p *Provider) IsBlank(c byte) (bool, error) { return false, ErrUnsupportedPlatform }

// IsCntrl always fails on the stub.
func (p *Provider) IsCntrl(c byte) (bool, error) { return false, ErrUnsupportedPlatform }

// IsDigit always fails on the stub.
func (p *Provider) IsDigit(c byte) (bool, error) { return false, ErrUnsupportedPlatform }

// IsGraph always fails on the stub.
func (p *Provider) IsGraph(c byte) (bool, error) { return false, ErrUnsupportedPlatform }

// IsLower always fails on the stub.
func (p *Provider) IsLower(c byte) (bool, error) { return false, ErrUnsupportedPlatform }

// IsPrint always fails on the stub.
func (p *Provider) IsPrint(c byte) (bool, error) { return false, ErrUnsupportedPlatform }

// IsPunct always fails on the stub.
func (p *Provider) IsPunct(c byte) (bool, error) { return false, ErrUnsupportedPlatform }

// IsSpace always fails on the stub.
func (p *Provider) IsSpace(c byte) (bool, error) { return false, ErrUnsupportedPlatform }

// IsUpper always fails on the stub.
func (p *Provider) IsUpper(c byte) (bool, error) { return false, ErrUnsupportedPlatform }

// IsXDigit always fails on the stub.
func (p *Provider) IsXDigit(c byte) (bool, error) { return false, ErrUnsupportedPlatform }

// ToLower always fails on the stub.
func (p *Provider) ToLower(b []byte) ([]byte, error) { return nil, ErrUnsupportedPlatform }

// ToUpper always fails on the stub.
func (p *Provider) ToUpper(b []byte) ([]byte, error) { return nil, ErrUnsupportedPlatform }

// Equivalents always fails on the stub.
func (p *Provider) Equivalents(c byte) ([]byte, error) { return nil, ErrUnsupportedPlatform }

// Close is a no-op on the stub.
func (p *Provider) Close() error { return nil }
