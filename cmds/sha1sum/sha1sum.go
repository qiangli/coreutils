// Package sha1sumcmd implements sha1sum(1) per the GNU coreutils
// manual. See cmds/md5sum for the family-wide semantics; the shared
// engine lives in cmds/internal/hashenc.
package sha1sumcmd

import (
	"crypto/sha1"

	"github.com/qiangli/coreutils/cmds/internal/hashenc"
	"github.com/qiangli/coreutils/tool"
)

var cmd = hashenc.NewSumTool(hashenc.SumSpec{
	Name: "sha1sum",
	Algo: "SHA1",
	Bits: 160,
	New:  sha1.New,
})

func init() { tool.Register(cmd) }
