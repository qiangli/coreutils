package nproccmd

import (
	"fmt"
	"math"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/qiangli/coreutils/tool"
)

var cmd = &tool.Tool{
	Name:     "nproc",
	Synopsis: "Print the number of processing units available.",
	Usage:    "nproc [OPTION]...",
}

func init() { cmd.Run = run; tool.Register(cmd) }

func run(rc *tool.RunContext, args []string) int {
	fs := tool.NewFlags(cmd.Name)
	all := fs.Bool("all", false, "print the number of installed processors")
	ignore := fs.Uint("ignore", 0, "exclude up to N processing units")
	operands, code := tool.Parse(rc, cmd, fs, tool.AliasHelpVersion(args))
	if code >= 0 {
		return code
	}
	if len(operands) > 0 {
		return tool.UsageError(rc, cmd, "extra operand %q", operands[0])
	}

	cores := runtime.NumCPU()
	if !*all {
		if omp := ompNumThreads(); omp > 0 {
			cores = omp
		} else if n := availableParallelism(); n > 0 {
			cores = n
		}
	}
	if *ignore >= uint(cores) {
		cores = 1
	} else {
		cores -= int(*ignore)
	}
	if limit := ompThreadLimit(); limit > 0 && cores > limit {
		cores = limit
	}
	if cores < 1 {
		cores = 1
	}
	fmt.Fprintln(rc.Out, cores)
	return 0
}

func availableParallelism() int {
	n, err := runtimeAvailableParallelism()
	if err != nil || n < 1 {
		return runtime.NumCPU()
	}
	return n
}

func runtimeAvailableParallelism() (int, error) {
	n, err := runtime.GOMAXPROCS(0), error(nil)
	if p, e := runtimeAvailable(); e == nil {
		n = p
	} else {
		err = e
	}
	if q := cgroupQuota(); q > 0 && q < n {
		n = q
	}
	return n, err
}

func runtimeAvailable() (int, error) {
	return runtime.GOMAXPROCS(0), nil
}

func ompNumThreads() int {
	s := os.Getenv("OMP_NUM_THREADS")
	if i := strings.IndexByte(s, ','); i >= 0 {
		s = s[:i]
	}
	n, err := strconv.ParseUint(strings.TrimSpace(s), 10, 0)
	if err != nil || n == 0 {
		return 0
	}
	if n > math.MaxInt {
		return math.MaxInt
	}
	return int(n)
}

func ompThreadLimit() int {
	n, err := strconv.ParseUint(strings.TrimSpace(os.Getenv("OMP_THREAD_LIMIT")), 10, 0)
	if err != nil || n == 0 {
		return 0
	}
	if n > math.MaxInt {
		return math.MaxInt
	}
	return int(n)
}
