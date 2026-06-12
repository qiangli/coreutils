// Package sha512sumcmd implements sha512sum(1) per the GNU coreutils
// manual. See cmds/md5sum for the family-wide semantics; the shared
// engine lives in cmds/internal/hashenc.
package sha512sumcmd

import (
	"crypto/sha512"

	"github.com/qiangli/coreutils/cmds/internal/hashenc"
	"github.com/qiangli/coreutils/tool"
)

var cmd = hashenc.NewSumTool(hashenc.SumSpec{
	Name: "sha512sum",
	Algo: "SHA512",
	Bits: 512,
	New:  sha512.New,
})

func init() { tool.Register(cmd) }
