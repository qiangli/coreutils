package factorcmd

import (
	"bufio"
	"fmt"
	"io"
	"math/big"
	"strconv"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "factor",
	Synopsis: "Print prime factors.",
	Usage:    "factor [OPTION] [NUMBER]...",
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	exponents := false
	var nums []string
	for _, a := range args {
		switch a {
		case "--help":
			fmt.Fprintf(rc.Out, "Usage: %s\n%s\n", cmd.Usage, cmd.Synopsis)
			return 0
		case "--version", "-V":
			fmt.Fprintf(rc.Out, "%s (qiangli/coreutils) %s\n", cmd.Name, tool.Version)
			return 0
		case "-h", "--exponents":
			exponents = true
		default:
			nums = append(nums, a)
		}
	}
	status := 0
	if len(nums) == 0 {
		data, err := io.ReadAll(rc.In)
		if err != nil {
			fmt.Fprintf(rc.Err, "factor: error reading input: %v\n", err)
			return 1
		}
		for _, f := range strings.FieldsFunc(string(data), func(r rune) bool { return r == ' ' || r == '\t' || r == '\n' || r == '\000' }) {
			if f != "" && !printFactors(rc, f, exponents) {
				status = 1
			}
		}
		return status
	}
	for _, n := range nums {
		if !printFactors(rc, strings.TrimSpace(n), exponents) {
			status = 1
		}
	}
	return status
}

func printFactors(rc *tool.RunContext, s string, exponents bool) bool {
	n, ok := new(big.Int).SetString(s, 10)
	if !ok || n.Sign() < 0 {
		fmt.Fprintf(rc.Err, "factor: %q is not a valid positive integer\n", s)
		return false
	}
	fmt.Fprintf(rc.Out, "%s:", n.String())
	if n.Sign() > 0 && n.BitLen() <= 64 {
		writeUintFactors(rc.Out, n.Uint64(), exponents)
	} else if n.Sign() > 1 {
		fmt.Fprintf(rc.Out, " %s", n.String())
	}
	fmt.Fprintln(rc.Out)
	return true
}

func writeUintFactors(w io.Writer, n uint64, exponents bool) {
	if n < 2 {
		return
	}
	counts := map[uint64]int{}
	order := []uint64{}
	add := func(p uint64) {
		if counts[p] == 0 {
			order = append(order, p)
		}
		counts[p]++
	}
	for n%2 == 0 {
		add(2)
		n /= 2
	}
	for p := uint64(3); p <= n/p; p += 2 {
		for n%p == 0 {
			add(p)
			n /= p
		}
	}
	if n > 1 {
		add(n)
	}
	for _, p := range order {
		if exponents && counts[p] > 1 {
			fmt.Fprintf(w, " %d^%d", p, counts[p])
		} else {
			for i := 0; i < counts[p]; i++ {
				fmt.Fprintf(w, " %d", p)
			}
		}
	}
}

var _ = bufio.ErrInvalidUnreadByte
var _ = strconv.IntSize
