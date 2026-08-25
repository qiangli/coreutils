//go:build linux || darwin

package sttycmd

import (
	"strings"

	"golang.org/x/sys/unix"
)

func platformOutputFlag(name string) (uint64, bool) {
	switch name {
	case "ofill":
		return unix.OFILL, true
	case "ofdel":
		return unix.OFDEL, true
	default:
		return 0, false
	}
}

func platformDelay(mode string) (uint64, uint64, bool) {
	switch mode {
	case "cr0":
		return unix.CRDLY, unix.CR0, true
	case "cr1":
		return unix.CRDLY, unix.CR1, true
	case "cr2":
		return unix.CRDLY, unix.CR2, true
	case "cr3":
		return unix.CRDLY, unix.CR3, true
	case "nl0":
		return unix.NLDLY, unix.NL0, true
	case "nl1":
		return unix.NLDLY, unix.NL1, true
	case "tab0", "tabs":
		return unix.TABDLY, unix.TAB0, true
	case "tab1":
		return unix.TABDLY, unix.TAB1, true
	case "tab2":
		return unix.TABDLY, unix.TAB2, true
	case "tab3":
		return unix.TABDLY, unix.TAB3, true
	case "bs0":
		return unix.BSDLY, unix.BS0, true
	case "bs1":
		return unix.BSDLY, unix.BS1, true
	case "ff0":
		return unix.FFDLY, unix.FF0, true
	case "ff1":
		return unix.FFDLY, unix.FF1, true
	case "vt0":
		return unix.VTDLY, unix.VT0, true
	case "vt1":
		return unix.VTDLY, unix.VT1, true
	default:
		return 0, 0, false
	}
}

func platformDelayReport(value uint64) string {
	groups := [][]string{
		{"cr0", "cr1", "cr2", "cr3"}, {"nl0", "nl1"},
		{"tab0", "tab1", "tab2", "tab3"}, {"bs0", "bs1"},
		{"ff0", "ff1"}, {"vt0", "vt1"},
	}
	names := make([]string, 0, len(groups))
	for _, group := range groups {
		selected := group[0]
		for _, name := range group {
			mask, candidate, _ := platformDelay(name)
			if value&mask == candidate {
				selected = name
				break
			}
		}
		names = append(names, selected)
	}
	return strings.Join(names, " ")
}
