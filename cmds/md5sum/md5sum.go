// Package md5sumcmd implements md5sum(1) per the GNU coreutils
// manual: print "<hex>  <file>" lines ("-" = stdin), -b switches the
// separator to " *" (digest bytes are identical on every platform —
// no text-mode translation ever happens), --tag emits BSD-style
// output, -c verifies checksum lists with OK/FAILED per line and GNU
// WARNING summaries. The engine lives in cmds/internal/hashenc and is
// shared with the sha*sum family.
package md5sumcmd

import (
	"crypto/md5"

	"github.com/qiangli/coreutils/cmds/internal/hashenc"
	"github.com/qiangli/coreutils/tool"
)

var cmd = hashenc.NewSumTool(hashenc.SumSpec{
	Name: "md5sum",
	Algo: "MD5",
	Bits: 128,
	New:  md5.New,
})

func init() { tool.Register(cmd) }
