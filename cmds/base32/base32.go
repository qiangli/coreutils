// Package base32cmd implements base32(1) per the GNU coreutils
// manual: encode with 76-column wrapping by default (-w COLS, 0 = no
// wrap), decode with -d (embedded newlines tolerated), -i to ignore
// non-alphabet bytes on decode. The engine lives in
// cmds/internal/hashenc and is shared with base64.
package base32cmd

import (
	"encoding/base32"
	"io"

	"github.com/qiangli/coreutils/cmds/internal/hashenc"
	"github.com/qiangli/coreutils/tool"
)

var cmd = hashenc.NewBaseTool(hashenc.BaseSpec{
	Name:    "base32",
	Display: "Base32",
	NewEncoder: func(w io.Writer) io.WriteCloser {
		return base32.NewEncoder(base32.StdEncoding, w)
	},
	NewDecoder: func(r io.Reader) io.Reader {
		return base32.NewDecoder(base32.StdEncoding, r)
	},
	InAlphabet: func(b byte) bool {
		return b >= 'A' && b <= 'Z' || b >= '2' && b <= '7'
	},
})

func init() { tool.Register(cmd) }
