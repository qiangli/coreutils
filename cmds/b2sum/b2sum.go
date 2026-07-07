// Package b2sumcmd implements b2sum(1) for BLAKE2b-512 checksums.
package b2sumcmd

import (
	"hash"

	"github.com/qiangli/coreutils/cmds/internal/hashenc"
	"github.com/qiangli/coreutils/tool"
	"golang.org/x/crypto/blake2b"
)

var cmd = hashenc.NewSumTool(hashenc.SumSpec{
	Name: "b2sum",
	Algo: "BLAKE2b",
	Bits: 512,
	New: func() hash.Hash {
		h, err := blake2b.New512(nil)
		if err != nil {
			panic(err)
		}
		return h
	},
})

func init() { tool.Register(cmd) }
