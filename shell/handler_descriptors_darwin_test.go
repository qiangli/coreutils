//go:build darwin

package shell

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"

	_ "github.com/qiangli/coreutils/cmds/env"
	_ "github.com/qiangli/coreutils/cmds/find"
	_ "github.com/qiangli/coreutils/cmds/tee"
	_ "github.com/qiangli/coreutils/cmds/timeout"
)

type descriptorOutput struct {
	sync.Mutex
	bytes.Buffer
}

func (b *descriptorOutput) Write(p []byte) (int, error) {
	b.Lock()
	defer b.Unlock()
	return b.Buffer.Write(p)
}
func (b *descriptorOutput) String() string { b.Lock(); defer b.Unlock(); return b.Buffer.String() }

func TestHandlerProcessSubstitutionDescriptors(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"env child", `env FD_TEST=yes /bin/cat <(echo env)`, "env\n"},
		{"timeout child", `timeout 3 /bin/cat <(echo timeout)`, "timeout\n"},
		{"find child", `find . -maxdepth 0 -exec /bin/cat <(echo found) \;`, "found\n"},
		{"cat", `cat <(printf 'hello\n')`, "hello\n"},
		{"head", `head -n 1 <(printf 'first\nsecond\n')`, "first\n"},
		{"tee", `printf 'hello\n' | tee >(cat) >/dev/null; wait`, "hello\n"},
		{"function", `f(){ /usr/bin/true; cat "$1"; cat "$1"; }; f <(echo once)`, "once\n"},
		{"closed", `f(){ n=${1##*/}; eval "exec $n<&-"; cat "$1" 2>/dev/null; echo status=$?; }; f <(echo unused)`, "status=1\n"},
		{"replacement", `f(){ n=${1##*/}; eval "exec $n</dev/null"; cat "$1"; echo replaced; }; f <(echo old)`, "replaced\n"},
		{"ordinary file", `printf 'file\n' > ordinary; cat ordinary`, "file\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			var out, errout descriptorOutput
			r, err := interp.New(interp.Dir(t.TempDir()), interp.StdIO(nil, &out, &errout), interp.ExecHandlers(Handler()))
			if err != nil {
				t.Fatal(err)
			}
			file, err := syntax.NewParser().Parse(strings.NewReader(tc.src), "")
			if err != nil {
				t.Fatal(err)
			}
			if err := r.Run(ctx, file); err != nil {
				t.Fatalf("run: %v stderr=%q", err, errout.String())
			}
			if got := out.String(); got != tc.want {
				t.Fatalf("got %q want %q stderr=%q", got, tc.want, errout.String())
			}
		})
	}
}
