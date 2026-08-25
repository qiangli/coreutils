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
	b, err := os.ReadFile("/proc/" + strconv.Itoa(p.pid) + "/stat")
	if err != nil {
		return
	}
	i := bytes.LastIndexByte(b, ')')
	if i < 0 {
		return
	}
	f := strings.Fields(string(b[i+2:]))
	if len(f) < 22 {
		return
	}
	p.pgid, _ = strconv.Atoi(f[2])
	p.sid, _ = strconv.Atoi(f[3])
	tty, _ := strconv.ParseUint(f[4], 10, 64)
	if tty != 0 {
		p.tty = ttyName(tty)
	}
	ut, _ := strconv.ParseInt(f[11], 10, 64)
	st, _ := strconv.ParseInt(f[12], 10, 64)
	p.cpu = time.Duration((ut + st) * int64(time.Second) / clockTicks())
	p.nice, _ = strconv.Atoi(f[16])
	p.vsz, _ = strconv.ParseUint(f[20], 10, 64)
	if status, err := os.ReadFile("/proc/" + strconv.Itoa(p.pid) + "/status"); err == nil {
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
