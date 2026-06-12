// Package sha224sumcmd implements sha224sum(1) per the GNU coreutils
// manual. See cmds/md5sum for the family-wide semantics; the shared
// engine lives in cmds/internal/hashenc.
package sha224sumcmd

import (
	"crypto/sha256"

	"github.com/qiangli/coreutils/cmds/internal/hashenc"
	"github.com/qiangli/coreutils/tool"
)

var cmd = hashenc.NewSumTool(hashenc.SumSpec{
	Name: "sha224sum",
	Algo: "SHA224",
	Bits: 224,
	New:  sha256.New224,
})

func init() { tool.Register(cmd) }
