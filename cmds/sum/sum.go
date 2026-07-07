// Package sumcmd implements the legacy sum(1) checksum utility.
package sumcmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "sum",
	Synopsis: "Print checksum and block count for each FILE. With no FILE, or when FILE is -, read standard input.",
	Usage:    "sum [OPTION]... [FILE]...",
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	bsd := fs.BoolP("bsd", "r", false, "use BSD checksum algorithm and 1K blocks")
	sysv := fs.BoolP("sysv", "s", false, "use System V checksum algorithm and 512-byte blocks")
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	if *bsd && *sysv {
		return tool.UsageError(rc, cmd, "cannot combine -r and -s")
	}
	if len(operands) == 0 {
		operands = []string{"-"}
	}
	exit := 0
	for _, name := range operands {
		result, err := sumOperand(rc, name, *sysv)
		if err != nil {
			fmt.Fprintf(rc.Err, "sum: %s: %s\n", name, errMsg(err))
			exit = 1
			continue
		}
		width := 5
		if *sysv {
			width = 1
		}
		if name == "-" {
			fmt.Fprintf(rc.Out, "%0*d %*d\n", width, result.checksum, width, result.blocks)
		} else {
			fmt.Fprintf(rc.Out, "%0*d %*d %s\n", width, result.checksum, width, result.blocks, name)
		}
	}
	return exit
}

type sumResult struct {
	checksum uint16
	blocks   uint64
}

func sumOperand(rc *tool.RunContext, name string, sysv bool) (sumResult, error) {
	var r io.Reader
	if name == "-" {
		if rc.In == nil {
			r = strings.NewReader("")
		} else {
			r = rc.In
		}
	} else {
		f, err := os.Open(rc.Path(name))
		if err != nil {
			return sumResult{}, err
		}
		defer f.Close()
		if fi, err := f.Stat(); err == nil && fi.IsDir() {
			return sumResult{}, errIsDirectory
		}
		r = f
	}
	if sysv {
		return sysvSum(r)
	}
	return bsdSum(r)
}

func bsdSum(r io.Reader) (sumResult, error) {
	var checksum uint16
	var size uint64
	buf := make([]byte, 32*1024)
	for {
		n, err := r.Read(buf)
		for _, b := range buf[:n] {
			checksum = (checksum >> 1) + ((checksum & 1) << 15) + uint16(b)
		}
		size += uint64(n)
		if err == io.EOF {
			break
		}
		if err != nil {
			return sumResult{}, err
		}
	}
	return sumResult{checksum: checksum, blocks: blocks(size, 1024)}, nil
}

func sysvSum(r io.Reader) (sumResult, error) {
	var checksum uint32
	var size uint64
	buf := make([]byte, 32*1024)
	for {
		n, err := r.Read(buf)
		for _, b := range buf[:n] {
			checksum += uint32(b)
		}
		size += uint64(n)
		if err == io.EOF {
			break
		}
		if err != nil {
			return sumResult{}, err
		}
	}
	checksum = (checksum & 0xffff) + (checksum >> 16)
	checksum = (checksum & 0xffff) + (checksum >> 16)
	return sumResult{checksum: uint16(checksum), blocks: blocks(size, 512)}, nil
}

func blocks(size, block uint64) uint64 {
	if size == 0 {
		return 0
	}
	return (size + block - 1) / block
}

var errIsDirectory = fmt.Errorf("Is a directory")

func errMsg(err error) string {
	if err == errIsDirectory {
		return err.Error()
	}
	if os.IsNotExist(err) {
		return "No such file or directory"
	}
	if os.IsPermission(err) {
		return "Permission denied"
	}
	return err.Error()
}
