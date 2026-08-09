//go:build !linux

package diffcmd

func (p *pathResolver) ensure() { p.tried = true }
