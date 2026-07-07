// Package cksumcmd implements POSIX cksum(1).
package cksumcmd

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "cksum",
	Synopsis: "Print POSIX CRC checksum and byte count for each FILE. With no FILE, or when FILE is -, read standard input.",
	Usage:    "cksum [OPTION]... [FILE]...",
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	operands, code := tool.Parse(rc, cmd, fs, args)
	if code >= 0 {
		return code
	}
	if len(operands) == 0 {
		operands = []string{"-"}
	}
	exit := 0
	for _, name := range operands {
		crc, size, err := cksumOperand(rc, name)
		if err != nil {
			fmt.Fprintf(rc.Err, "cksum: %s: %s\n", name, errMsg(err))
			exit = 1
			continue
		}
		if name == "-" {
			fmt.Fprintf(rc.Out, "%d %d\n", crc, size)
		} else {
			fmt.Fprintf(rc.Out, "%d %d %s\n", crc, size, name)
		}
	}
	return exit
}

func cksumOperand(rc *tool.RunContext, name string) (uint32, uint64, error) {
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
			return 0, 0, err
		}
		defer f.Close()
		if fi, err := f.Stat(); err == nil && fi.IsDir() {
			return 0, 0, errIsDirectory
		}
		r = f
	}
	var crc uint32
	var n uint64
	buf := make([]byte, 32*1024)
	for {
		got, err := r.Read(buf)
		for _, b := range buf[:got] {
			crc = (crc << 8) ^ cksumTable[((crc>>24)^uint32(b))&0xff]
		}
		n += uint64(got)
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, 0, err
		}
	}
	for length := n; length != 0; length >>= 8 {
		crc = (crc << 8) ^ cksumTable[((crc>>24)^uint32(length&0xff))&0xff]
	}
	return ^crc, n, nil
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

var cksumTable = makeCKSumTable()

func makeCKSumTable() [256]uint32 {
	const poly uint32 = 0x04c11db7
	var tab [256]uint32
	for i := range tab {
		crc := uint32(i) << 24
		for j := 0; j < 8; j++ {
			if crc&0x80000000 != 0 {
				crc = (crc << 1) ^ poly
			} else {
				crc <<= 1
			}
		}
		tab[i] = crc
	}
	return tab
}
