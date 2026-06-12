// Package base64cmd implements base64(1) per the GNU coreutils
// manual: encode with 76-column wrapping by default (-w COLS, 0 = no
// wrap), decode with -d (embedded newlines tolerated), -i to ignore
// non-alphabet bytes on decode. The engine lives in
// cmds/internal/hashenc and is shared with base32.
package base64cmd

import (
	"encoding/base64"
	"io"

	"github.com/qiangli/coreutils/cmds/internal/hashenc"
	"github.com/qiangli/coreutils/tool"
)

var cmd = hashenc.NewBaseTool(hashenc.BaseSpec{
	Name:    "base64",
	Display: "Base64",
	NewEncoder: func(w io.Writer) io.WriteCloser {
		return base64.NewEncoder(base64.StdEncoding, w)
	},
	NewDecoder: func(r io.Reader) io.Reader {
		return base64.NewDecoder(base64.StdEncoding, r)
	},
	InAlphabet: func(b byte) bool {
		return b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' ||
			b >= '0' && b <= '9' || b == '+' || b == '/'
	},
})

func init() { tool.Register(cmd) }
