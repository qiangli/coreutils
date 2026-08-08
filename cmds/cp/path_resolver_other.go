//go:build !linux

package cpcmd

func (p *pathResolver) ensure() { p.tried = true }
