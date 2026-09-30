package collate

import "github.com/qiangli/coreutils/pkg/locale"

// BracketElements returns a detached snapshot for full-element bracket
// consumers. The legacy byte-table methods deliberately remain lossless-only.
func (p *compiledProvider) BracketElements() ([]locale.CollatingElement, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.closed {
		return nil, ErrClosed
	}
	out := make([]locale.CollatingElement, len(p.elements))
	for i, e := range p.elements {
		out[i] = e
		out[i].Weights = make([][]int, len(e.Weights))
		for j, weights := range e.Weights {
			out[i].Weights[j] = append([]int(nil), weights...)
		}
	}
	return out, nil
}
