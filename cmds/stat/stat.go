// Package statcmd implements stat(1) per the GNU coreutils manual:
// the default information block, plus --format/-c with the directive
// subset %n %s %F %a %U %G %u %g %x %y %z %i %h (and %%). Directives
// outside the subset fail with a clear error rather than a guess, per
// the repository contract. Like GNU stat, symlinks are not followed
// (the link itself is reported).
//
// Platform note: on Windows there is no inode / link count / uid /
// gid / block count — they report 0 / 1 / 0 / 0 / a size-derived
// value; owner and group names are a best-effort SID account lookup
// ("UNKNOWN" when unavailable, matching GNU's unresolvable-ID
// spelling); the change time (%z) reports the last write time, and
// the birth time comes from the file's creation time.
package statcmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "stat",
	Synopsis: "Display file or file system status.",
	Usage:    "stat [OPTION]... FILE...",
}

// Run is wired in init: a literal would create an initialization
// cycle (run's flag-error paths reference cmd).
func init() { cmd.Run = run; tool.Register(cmd) }

// fileMeta is one file's gathered metadata; the platform-dependent
// fields are filled by fillSys (sys_linux.go / sys_darwin.go /
// sys_windows.go).
type fileMeta struct {
	name, target     string
	size             int64
	blocks           int64
	ioBlock          int64
	fileType         string
	permBits         uint32
	modeStr          string
	uid, gid         uint32
	uname, gname     string
	devMaj, devMin   uint32
	rdevMaj, rdevMin uint32
	isDevice         bool
	ino              uint64
	nlink            uint64
	atime, mtime     time.Time
	ctime, birth     time.Time
	hasBirth         bool
}

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	format := fs.StringP("format", "c", "", "use the specified FORMAT instead of the default; output a newline after each use of FORMAT")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	if len(operands) == 0 {
		return tool.UsageError(rc, cmd, "missing operand")
	}
	useFormat := fs.Changed("format")
	if useFormat {
		if c := checkFormat(rc, *format); c >= 0 {
			return c
		}
	}

	exit := 0
	for _, op := range operands {
		full := rc.Path(op)
		fi, err := os.Lstat(full)
		if err != nil {
			fmt.Fprintf(rc.Err, "stat: cannot stat '%s': %s\n", op, errMsg(err))
			exit = 1
			continue
		}
		m := gather(full, op, fi)
		if useFormat {
			expandFormat(rc.Out, *format, m)
		} else {
			printDefault(rc.Out, m)
		}
	}
	return exit
}

func gather(path, name string, fi os.FileInfo) *fileMeta {
	m := &fileMeta{
		name:     name,
		size:     fi.Size(),
		mtime:    fi.ModTime(),
		modeStr:  modeString(fi.Mode()),
		permBits: permBits(fi.Mode()),
		fileType: fileTypeName(fi),
		isDevice: fi.Mode()&os.ModeDevice != 0,
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		m.target, _ = os.Readlink(path)
	}
	fillSys(m, path, fi)
	return m
}

// supported is the -c directive subset.
const supported = "%nsFaUGugxyzih"

// checkFormat validates the format once up front so a bad directive
// fails before any output. Returns -1 when the format is acceptable.
func checkFormat(rc *tool.RunContext, format string) int {
	for i := 0; i < len(format); i++ {
		if format[i] != '%' {
			continue
		}
		i++
		if i >= len(format) {
			break // trailing % is emitted literally, like GNU
		}
		if strings.IndexByte(supported, format[i]) < 0 {
			return tool.NotSupported(rc, cmd, fmt.Sprintf("format directive '%%%c'", format[i]))
		}
	}
	return -1
}

func expandFormat(w io.Writer, format string, m *fileMeta) {
	var b strings.Builder
	for i := 0; i < len(format); i++ {
		c := format[i]
		if c != '%' {
			b.WriteByte(c)
			continue
		}
		i++
		if i >= len(format) {
			b.WriteByte('%')
			break
		}
		switch format[i] {
		case '%':
			b.WriteByte('%')
		case 'n':
			b.WriteString(m.name)
		case 's':
			b.WriteString(strconv.FormatInt(m.size, 10))
		case 'F':
			b.WriteString(m.fileType)
		case 'a':
			b.WriteString(strconv.FormatUint(uint64(m.permBits), 8))
		case 'U':
			b.WriteString(m.uname)
		case 'G':
			b.WriteString(m.gname)
		case 'u':
			b.WriteString(strconv.FormatUint(uint64(m.uid), 10))
		case 'g':
			b.WriteString(strconv.FormatUint(uint64(m.gid), 10))
		case 'x':
			b.WriteString(tsString(m.atime))
		case 'y':
			b.WriteString(tsString(m.mtime))
		case 'z':
			b.WriteString(tsString(m.ctime))
		case 'i':
			b.WriteString(strconv.FormatUint(m.ino, 10))
		case 'h':
			b.WriteString(strconv.FormatUint(m.nlink, 10))
		}
	}
	fmt.Fprintln(w, b.String())
}

// printDefault renders the GNU default information block.
func printDefault(w io.Writer, m *fileMeta) {
	name := m.name
	if m.target != "" {
		name += " -> " + m.target
	}
	fmt.Fprintf(w, "  File: %s\n", name)
	fmt.Fprintf(w, "  Size: %-10d\tBlocks: %-10d IO Block: %-6d %s\n",
		m.size, m.blocks, m.ioBlock, m.fileType)
	if m.isDevice {
		fmt.Fprintf(w, "Device: %d,%d\tInode: %-10d  Links: %-5d Device type: %d,%d\n",
			m.devMaj, m.devMin, m.ino, m.nlink, m.rdevMaj, m.rdevMin)
	} else {
		fmt.Fprintf(w, "Device: %d,%d\tInode: %-10d  Links: %d\n",
			m.devMaj, m.devMin, m.ino, m.nlink)
	}
	fmt.Fprintf(w, "Access: (%04o/%s)  Uid: (%5d/%8s)   Gid: (%5d/%8s)\n",
		m.permBits, m.modeStr, m.uid, m.uname, m.gid, m.gname)
	fmt.Fprintf(w, "Access: %s\n", tsString(m.atime))
	fmt.Fprintf(w, "Modify: %s\n", tsString(m.mtime))
	fmt.Fprintf(w, "Change: %s\n", tsString(m.ctime))
	if m.hasBirth {
		fmt.Fprintf(w, " Birth: %s\n", tsString(m.birth))
	} else {
		fmt.Fprintf(w, " Birth: -\n")
	}
}

func tsString(t time.Time) string {
	return t.Format("2006-01-02 15:04:05.000000000 -0700")
}

func fileTypeName(fi os.FileInfo) string {
	m := fi.Mode()
	switch {
	case m&os.ModeSymlink != 0:
		return "symbolic link"
	case m.IsDir():
		return "directory"
	case m&os.ModeCharDevice != 0:
		return "character special file"
	case m&os.ModeDevice != 0:
		return "block special file"
	case m&os.ModeNamedPipe != 0:
		return "fifo"
	case m&os.ModeSocket != 0:
		return "socket"
	case fi.Size() == 0:
		return "regular empty file"
	default:
		return "regular file"
	}
}

// permBits is the full octal access-rights value (%a): permission
// bits plus setuid/setgid/sticky.
func permBits(m os.FileMode) uint32 {
	bits := uint32(m.Perm())
	if m&os.ModeSetuid != 0 {
		bits |= 0o4000
	}
	if m&os.ModeSetgid != 0 {
		bits |= 0o2000
	}
	if m&os.ModeSticky != 0 {
		bits |= 0o1000
	}
	return bits
}

// modeString builds the GNU 10-character permission string.
func modeString(m os.FileMode) string {
	b := []byte("----------")
	switch {
	case m&os.ModeDir != 0:
		b[0] = 'd'
	case m&os.ModeSymlink != 0:
		b[0] = 'l'
	case m&os.ModeCharDevice != 0:
		b[0] = 'c'
	case m&os.ModeDevice != 0:
		b[0] = 'b'
	case m&os.ModeNamedPipe != 0:
		b[0] = 'p'
	case m&os.ModeSocket != 0:
		b[0] = 's'
	}
	const rwx = "rwxrwxrwx"
	perm := m.Perm()
	for i := 0; i < 9; i++ {
		if perm&(1<<uint(8-i)) != 0 {
			b[i+1] = rwx[i]
		}
	}
	if m&os.ModeSetuid != 0 {
		if b[3] == 'x' {
			b[3] = 's'
		} else {
			b[3] = 'S'
		}
	}
	if m&os.ModeSetgid != 0 {
		if b[6] == 'x' {
			b[6] = 's'
		} else {
			b[6] = 'S'
		}
	}
	if m&os.ModeSticky != 0 {
		if b[9] == 'x' {
			b[9] = 't'
		} else {
			b[9] = 'T'
		}
	}
	return string(b)
}

func errMsg(err error) string {
	var pe *os.PathError
	if errors.As(err, &pe) {
		return pe.Err.Error()
	}
	return err.Error()
}
