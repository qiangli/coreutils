//go:build linux

package cpcmd

import (
	"fmt"
	"os"
)

func (p *pathResolver) ensure() {
	if p.tried {
		return
	}
	p.tried = true
	dir, err := os.Open(p.rc.Dir)
	if err != nil {
		return
	}
	p.dir = dir
	p.prefix = fmt.Sprintf("/proc/self/fd/%d", dir.Fd())
	if _, err := os.Stat(p.prefix); err != nil {
		p.close()
	}
}
