// Package sha256sumcmd implements sha256sum(1) per the GNU coreutils
// manual. See cmds/md5sum for the family-wide semantics; the shared
// engine lives in cmds/internal/hashenc.
package sha256sumcmd

import (
	"crypto/sha256"

	"github.com/qiangli/coreutils/cmds/internal/hashenc"
	"github.com/qiangli/coreutils/tool"
)

var cmd = hashenc.NewSumTool(hashenc.SumSpec{
	Name: "sha256sum",
	Algo: "SHA256",
	Bits: 256,
	New:  sha256.New,
})

func init() { tool.Register(cmd) }
