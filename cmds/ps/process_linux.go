//go:build linux

package pscmd

import (
	"bytes"
	"os"
	"strconv"
	"strings"
	"time"
)

func enrich(p *process) {
	enrichWithReader(p, os.ReadFile)
}

func enrichWithReader(p *process, readFile func(string) ([]byte, error)) {
	root := "/proc/" + strconv.Itoa(p.pid)
	b, err := readFile(root + "/stat")
	if err != nil {
		return
	}
	i := bytes.LastIndexByte(b, ')')
	if i < 0 {
		return
	}
	f := strings.Fields(string(b[i+2:]))
	if len(f) < 24 {
		return
	}
	p.state = f[0]
	if ppid, err := strconv.Atoi(f[1]); err == nil {
		p.ppid = ppid
	}
	if pgid, err := strconv.Atoi(f[2]); err == nil {
		p.pgid, p.pgidKnown = pgid, true
	}
	if sid, err := strconv.Atoi(f[3]); err == nil {
		p.sid = sid
	}
	tty, _ := strconv.ParseUint(f[4], 10, 64)
	if tty != 0 {
		p.tty = ttyName(tty)
	}
	ut, utErr := strconv.ParseInt(f[11], 10, 64)
	st, stErr := strconv.ParseInt(f[12], 10, 64)
	if utErr == nil && stErr == nil {
		p.cpu = time.Duration((ut + st) * int64(time.Second) / clockTicks())
		p.cpuKnown = true
	}
	if flags, err := strconv.ParseUint(f[6], 10, 64); err == nil {
		p.flags, p.flagsKnown = flags, true
	}
	if priority, err := strconv.Atoi(f[15]); err == nil {
		p.priority, p.priorityKnown = priority, true
	}
	if nice, err := strconv.Atoi(f[16]); err == nil {
		p.nice, p.niceKnown = nice, true
	}
	if vsz, err := strconv.ParseUint(f[20], 10, 64); err == nil {
		p.vsz, p.vszKnown = vsz, true
	}
	if statm, err := readFile(root + "/statm"); err == nil {
		if values := strings.Fields(string(statm)); len(values) != 0 {
			if sz, err := strconv.ParseUint(values[0], 10, 64); err == nil {
				p.sz, p.szKnown = sz, true
			}
		}
	}
	if addr, err := strconv.ParseUint(f[23], 10, 64); err == nil && addr != 0 {
		p.addr, p.addrKnown = addr, true
	}
	if wchan, err := readFile(root + "/wchan"); err == nil {
		p.wchan = strings.TrimSpace(string(wchan))
	}
	if status, err := readFile(root + "/status"); err == nil {
		for _, line := range strings.Split(string(status), "\n") {
			fields := strings.Fields(line)
			switch {
			case len(fields) >= 3 && fields[0] == "Uid:":
				p.ruid, _ = strconv.Atoi(fields[1])
				p.euid, _ = strconv.Atoi(fields[2])
			case len(fields) >= 3 && fields[0] == "Gid:":
				p.rgid, _ = strconv.Atoi(fields[1])
				p.egid, _ = strconv.Atoi(fields[2])
			}
		}
	}
}

func ttyName(dev uint64) string {
	minor := dev&0xff | (dev >> 12 & 0xfff00)
	major := dev >> 8 & 0xfff
	if major == 136 || major == 3 {
		return "pts/" + strconv.FormatUint(minor, 10)
	}
	return "tty" + strconv.FormatUint(minor, 10)
}
func clockTicks() int64 { return 100 }
func currentUID() int   { return os.Geteuid() }
func currentTTY() string {
	p := process{pid: os.Getpid()}
	enrich(&p)
	return p.tty
}
