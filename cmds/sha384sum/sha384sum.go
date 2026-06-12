// Package sha384sumcmd implements sha384sum(1) per the GNU coreutils
// manual. See cmds/md5sum for the family-wide semantics; the shared
// engine lives in cmds/internal/hashenc.
package sha384sumcmd

import (
	"crypto/sha512"

	"github.com/qiangli/coreutils/cmds/internal/hashenc"
	"github.com/qiangli/coreutils/tool"
)

var cmd = hashenc.NewSumTool(hashenc.SumSpec{
	Name: "sha384sum",
	Algo: "SHA384",
	Bits: 384,
	New:  sha512.New384,
})

func init() { tool.Register(cmd) }
