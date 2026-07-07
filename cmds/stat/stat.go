// Package statcmd implements stat(1) per the GNU coreutils manual:
// the default information block, plus --format/-c --printf --terse/-t
// --dereference/-L --file-system/-f with directives %n %s %F %a %U %G
// %u %g %x %y %z %i %h (and %%). Directives outside the subset fail
// with a clear error.
//
// Platform note: on Windows there is no inode / link count / uid /
// gid / block count — they report 0 / 1 / 0 / 0 / a size-derived
// value; owner and group names are a best-effort SID account lookup.
package statcmd

import (
	"fmt"
	"io"
	"os"
	"runtime"
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

func init() { cmd.Run = run; tool.Register(cmd) }

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
	dereference := fs.BoolP("dereference", "L", false, "follow links")
	fileSystem := fs.BoolP("file-system", "f", false, "display file system status instead of file status")
	printf := fs.StringP("printf", "", "", "like --format, but interpret backslash escapes and do not output a mandatory trailing newline")
	terse := fs.BoolP("terse", "t", false, "print the information in terse form")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	if len(operands) == 0 {
		return tool.UsageError(rc, cmd, "missing operand")
	}

	useFormat := fs.Changed("format")
	usePrintf := fs.Changed("printf")

	if useFormat {
		if c := checkFormat(rc, *format); c >= 0 {
			return c
		}
	}
	if usePrintf {
		if c := checkFormat(rc, *printf); c >= 0 {
			return c
		}
	}

	exit := 0
	for _, op := range operands {
		full := rc.Path(op)
		if *fileSystem {
			if d := showFileSystem(rc, full, op, *terse); d != 0 {
				exit = 1
			}
			continue
		}

		var fi os.FileInfo
		var m *fileMeta
		if *dereference {
			var err error
			fi, err = os.Stat(full)
			if err != nil {
				fmt.Fprintf(rc.Err, "stat: cannot stat '%s': %s\n", op, errMsg(err))
				exit = 1
				continue
			}
			m = gather(full, op, fi)
		} else {
			var err error
			fi, err = os.Lstat(full)
			if err != nil {
				fmt.Fprintf(rc.Err, "stat: cannot stat '%s': %s\n", op, errMsg(err))
				exit = 1
				continue
			}
			m = gatherNoFollow(full, op, fi)
		}

		if *terse {
			printTerse(rc.Out, m)
		} else if usePrintf {
			fmt.Fprint(rc.Out, expandFormatStr(rc, *printf, m))
		} else if useFormat {
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
	fillSys(m, path, fi)
	return m
}

func gatherNoFollow(path, name string, fi os.FileInfo) *fileMeta {
	m := gather(path, name, fi)
	if fi.Mode()&os.ModeSymlink != 0 {
		m.target, _ = os.Readlink(path)
	}
	return m
}

const supported = "%nsFaUGugxyzih"

func checkFormat(rc *tool.RunContext, format string) int {
	for i := 0; i < len(format); i++ {
		if format[i] != '%' {
			continue
		}
		i++
		if i >= len(format) {
			break
		}
		if strings.IndexByte(supported, format[i]) < 0 {
			return tool.NotSupported(rc, cmd, fmt.Sprintf("format directive '%%%c'", format[i]))
		}
	}
	return -1
}

func expandFormat(w io.Writer, format string, m *fileMeta) {
	fmt.Fprintln(w, expandFormatStr(nil, format, m))
}

func expandFormatStr(rc *tool.RunContext, format string, m *fileMeta) string {
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
	return b.String()
}

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

func printTerse(w io.Writer, m *fileMeta) {
	fmt.Fprintf(w, "%s %d %d %03o %d %d %x %x %d %d %d %d %d %d %d %d\n",
		m.name, m.size, m.blocks, m.permBits, m.uid, m.gid,
		m.devMaj, m.devMin, m.ino, m.nlink, m.rdevMaj, m.rdevMin,
		m.atime.Unix(), m.mtime.Unix(), m.ctime.Unix(), 0)
}

func showFileSystem(rc *tool.RunContext, path, op string, terse bool) int {
	if runtime.GOOS == "windows" {
		notImplemente(rc, op)
		return 1
	}
	st, err := statfsFile(path)
	if err != nil {
		fmt.Fprintf(rc.Err, "stat: cannot read file system information for '%s': %s\n", op, err)
		return 1
	}
	if terse {
		fmt.Fprintf(rc.Out, "%s %d %d %d %d %d %d %d\n",
			op, st.blockSize, st.totalBlocks, st.freeBlocks,
			st.availBlocks, st.totalInodes, st.freeInodes, st.nameMax)
	} else {
		fmt.Fprintf(rc.Out, "  File: \"%s\"\n", op)
		fmt.Fprintf(rc.Out, "    ID: %x Namelen: %-5d Type: %s\n", 0, st.nameMax, st.fsType)
		fmt.Fprintf(rc.Out, "Block size: %-10d Fundamental block size: %d\n", st.blockSize, st.blockSize)
		fmt.Fprintf(rc.Out, "Blocks: Total: %-10d Free: %-10d Available: %d\n",
			st.totalBlocks, st.freeBlocks, st.availBlocks)
		fmt.Fprintf(rc.Out, "Inodes: Total: %-10d Free: %d\n", st.totalInodes, st.freeInodes)
	}
	return 0
}

func notImplemente(rc *tool.RunContext, op string) {
	fmt.Fprintf(rc.Err, "stat: --file-system is not supported on this platform for '%s'\n", op)
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
	return tool.SysErrString(err)
}
