//go:build linux

package pscmd

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
)

type linuxTiming struct {
	boot  time.Time
	ticks int64
	known bool
}

func prepareProcessSource() (func(*process) bool, error) {
	timing := linuxTimingFromReader(os.ReadFile)
	return func(p *process) bool { return enrichWithReaderAndTiming(p, os.ReadFile, timing) }, nil
}

func enrich(p *process) bool {
	return enrichWithReader(p, os.ReadFile)
}

func enrichWithReader(p *process, readFile func(string) ([]byte, error)) bool {
	return enrichWithReaderAndTiming(p, readFile, linuxTimingFromReader(readFile))
}

func enrichWithReaderAndTiming(p *process, readFile func(string) ([]byte, error), timing linuxTiming) bool {
	root := "/proc/" + strconv.Itoa(p.pid)
	b, err := readFile(root + "/stat")
	if err != nil {
		return false
	}
	commStart := bytes.IndexByte(b, '(')
	i := bytes.LastIndexByte(b, ')')
	if commStart < 0 || i <= commStart {
		return false
	}
	p.command = string(b[commStart+1 : i])
	p.args = p.command
	f := strings.Fields(string(b[i+1:]))
	if len(f) < 24 {
		return false
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
	tty, ttyErr := strconv.ParseInt(f[4], 10, 64)
	if ttyErr == nil && tty != 0 {
		p.tty = ttyNameWithReader(uint64(uint32(tty)), readFile)
	}
	ut, utErr := strconv.ParseUint(f[11], 10, 64)
	st, stErr := strconv.ParseUint(f[12], 10, 64)
	if utErr == nil && stErr == nil && ut <= math.MaxUint64-st && timing.known {
		p.cpu, p.cpuKnown = durationFromClockTicks(ut+st, timing.ticks)
	}
	if started, err := strconv.ParseUint(f[19], 10, 64); err == nil && timing.known {
		if sinceBoot, ok := durationFromClockTicks(started, timing.ticks); ok {
			p.start = timing.boot.Add(sinceBoot)
		}
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
		p.wchanKnown = p.wchan != ""
	}
	if cmdline, err := readFile(root + "/cmdline"); err == nil && len(cmdline) != 0 {
		if cmdline[len(cmdline)-1] == 0 {
			cmdline = cmdline[:len(cmdline)-1]
		}
		argv := bytes.Split(cmdline, []byte{0})
		p.command = string(argv[0]) // POSIX comm is argv[0], including an empty argv[0].
		parts := make([]string, len(argv))
		for i := range argv {
			parts[i] = string(argv[i])
		}
		p.args = strings.Join(parts, " ")
		p.argvKnown = true
	}
	if status, err := readFile(root + "/status"); err == nil {
		for _, line := range strings.Split(string(status), "\n") {
			fields := strings.Fields(line)
			switch {
			case len(fields) >= 3 && fields[0] == "Uid:":
				ruid, rerr := strconv.Atoi(fields[1])
				euid, eerr := strconv.Atoi(fields[2])
				if rerr == nil && eerr == nil {
					p.ruid, p.euid = ruid, euid
				}
			case len(fields) >= 3 && fields[0] == "Gid:":
				rgid, rerr := strconv.Atoi(fields[1])
				egid, eerr := strconv.Atoi(fields[2])
				if rerr == nil && eerr == nil {
					p.rgid, p.egid = rgid, egid
				}
			}
		}
	}
	return true
}

func ttyName(dev uint64) string {
	return ttyNameWithReader(dev, os.ReadFile)
}

func ttyNameWithReader(dev uint64, readFile func(string) ([]byte, error)) string {
	minor := dev&0xff | (dev >> 12 & 0xfff00)
	major := dev >> 8 & 0xfff
	if major >= 136 && major <= 143 {
		index := (major-136)*256 + minor
		return "pts/" + strconv.FormatUint(index, 10)
	}
	if major == 4 && minor < 64 {
		return "tty" + strconv.FormatUint(minor, 10)
	}
	uevent, err := readFile("/sys/dev/char/" + strconv.FormatUint(major, 10) + ":" + strconv.FormatUint(minor, 10) + "/uevent")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(uevent), "\n") {
		if name, ok := strings.CutPrefix(line, "DEVNAME="); ok {
			return strings.TrimPrefix(name, "/dev/")
		}
	}
	return ""
}

func linuxTimingFromReader(readFile func(string) ([]byte, error)) linuxTiming {
	stat, statErr := readFile("/proc/stat")
	auxv, auxvErr := readFile("/proc/self/auxv")
	if statErr != nil || auxvErr != nil {
		return linuxTiming{}
	}
	var bootSeconds int64
	for _, line := range strings.Split(string(stat), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "btime" {
			bootSeconds, statErr = strconv.ParseInt(fields[1], 10, 64)
			break
		}
	}
	if statErr != nil || bootSeconds <= 0 {
		return linuxTiming{}
	}
	word := strconv.IntSize / 8
	for off := 0; off+2*word <= len(auxv); off += 2 * word {
		var tag, value uint64
		if word == 8 {
			tag = binary.NativeEndian.Uint64(auxv[off:])
			value = binary.NativeEndian.Uint64(auxv[off+word:])
		} else {
			tag = uint64(binary.NativeEndian.Uint32(auxv[off:]))
			value = uint64(binary.NativeEndian.Uint32(auxv[off+word:]))
		}
		// AT_CLKTCK is a frequency. This upper bound is far above Linux's
		// supported USER_HZ values and keeps the nanosecond conversion below
		// from overflowing before division.
		if tag == 17 && value > 0 && value <= uint64(math.MaxInt64/int64(time.Second)) {
			return linuxTiming{boot: time.Unix(bootSeconds, 0), ticks: int64(value), known: true}
		}
	}
	return linuxTiming{}
}

func durationFromClockTicks(ticks uint64, hz int64) (time.Duration, bool) {
	if hz <= 0 {
		return 0, false
	}
	frequency := uint64(hz)
	seconds, remainder := ticks/frequency, ticks%frequency
	maxSeconds := uint64(math.MaxInt64 / int64(time.Second))
	if seconds > maxSeconds {
		return 0, false
	}
	d := time.Duration(seconds) * time.Second
	fraction := time.Duration(remainder) * time.Second / time.Duration(frequency)
	if d > time.Duration(math.MaxInt64)-fraction {
		return 0, false
	}
	return d + fraction, true
}

func currentUID() int { return os.Geteuid() }
func currentTTY() string {
	p := process{pid: os.Getpid()}
	_ = enrich(&p)
	return p.tty
}
