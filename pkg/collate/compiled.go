package collate

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/qiangli/coreutils/pkg/locale"
)

// Engine is the shared compiled/host collation contract. Byte-table consumers
// receive an explicit error for orders requiring multibyte or multi-character
// matching, rather than a lossy table.
type Engine interface {
	Compare(string, string) (int, error)
	Close() error
	Equivalents(byte) ([]byte, error)
	EquivalenceClasses() ([]bool, error)
	CollationWeights() ([]byte, error)
	CollatingElements() ([]bool, error)
}

// OpenEnv gives compiled locales precedence and never hides malformed stores.
// env must carry invocation-resolved store paths (locale.StoreEnvAt).
func OpenEnv(env []string, name string) (Engine, error) {
	for _, path := range locale.CompiledPaths(env, name) {
		c, err := locale.LoadSelected(path, name)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if c.Collation == nil {
			return nil, fmt.Errorf("collate: compiled locale %q has no LC_COLLATE", name)
		}
		p := &compiledProvider{data: c.Collation}
		p.elements = append([]locale.CollatingElement(nil), c.Collation.Elements...)
		slices.SortFunc(p.elements, func(a, b locale.CollatingElement) int { return len(b.Text) - len(a.Text) })
		return p, nil
	}
	return Open(name)
}

type compiledProvider struct {
	mu       sync.RWMutex
	closed   bool
	data     *locale.Collation
	elements []locale.CollatingElement
}

func (p *compiledProvider) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	return nil
}
func (p *compiledProvider) sequence(s string) ([][]int, error) {
	result := make([][]int, len(p.data.Backward))
	for len(s) > 0 {
		found := false
		for _, e := range p.elements {
			if strings.HasPrefix(s, e.Text) {
				for i, w := range e.Weights {
					result[i] = append(result[i], w...)
				}
				s = s[len(e.Text):]
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("collate: input contains an undefined collating element at %q", s)
		}
	}
	for i, backward := range p.data.Backward {
		if backward {
			slices.Reverse(result[i])
		}
	}
	return result, nil
}
func (p *compiledProvider) Compare(a, b string) (int, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.closed {
		return 0, ErrClosed
	}
	x, err := p.sequence(a)
	if err != nil {
		return 0, err
	}
	y, err := p.sequence(b)
	if err != nil {
		return 0, err
	}
	for i := range x {
		if n := slices.Compare(x[i], y[i]); n != 0 {
			return n, nil
		}
	}
	return 0, nil
}
func (p *compiledProvider) byteElements() ([]locale.CollatingElement, error) {
	if p.closed {
		return nil, ErrClosed
	}
	es := append([]locale.CollatingElement(nil), p.elements...)
	for _, e := range es {
		if len(e.Text) != 1 {
			return nil, fmt.Errorf("collate: byte bracket expressions do not support multibyte or multi-character collating elements")
		}
	}
	slices.SortFunc(es, func(a, b locale.CollatingElement) int { return a.Order - b.Order })
	return es, nil
}
func (p *compiledProvider) Equivalents(c byte) ([]byte, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	es, err := p.byteElements()
	if err != nil {
		return nil, err
	}
	var target *locale.CollatingElement
	for i := range es {
		if es[i].Text[0] == c {
			target = &es[i]
			break
		}
	}
	if target == nil {
		return nil, nil
	}
	var out []byte
	for _, e := range es {
		if slices.Equal(target.Weights[0], e.Weights[0]) {
			out = append(out, e.Text[0])
		}
	}
	return out, nil
}
func (p *compiledProvider) CollatingElements() ([]bool, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	es, err := p.byteElements()
	if err != nil {
		return nil, err
	}
	out := make([]bool, 256)
	for _, e := range es {
		out[e.Text[0]] = true
	}
	return out, nil
}
func (p *compiledProvider) EquivalenceClasses() ([]bool, error) { return p.CollatingElements() }
func (p *compiledProvider) CollationWeights() ([]byte, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	es, err := p.byteElements()
	if err != nil {
		return nil, err
	}
	out := make([]byte, 256)
	for i, e := range es {
		out[e.Text[0]] = byte(i)
	}
	return out, nil
}
