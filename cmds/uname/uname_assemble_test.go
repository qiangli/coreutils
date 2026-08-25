package unamecmd

import (
	"reflect"
	"testing"
)

// A synthetic or failed probe can lack a version value. Assembly must not
// introduce an empty output column, although every supported platform probe
// is required to provide a non-empty POSIX -v symbol.
func TestAssembleSkipsSyntheticEmptyVersion(t *testing.T) {
	info := sysinfo{sysname: "S", nodename: "N", release: "R", version: "", machine: "M"}
	want := []string{"S", "N", "R", "M"}
	if got := assemble(info, selection{all: true}); !reflect.DeepEqual(got, want) {
		t.Errorf("-a: %v, want %v", got, want)
	}
	if got := assemble(info, selection{all: true, version: true}); !reflect.DeepEqual(got, want) {
		t.Errorf("-a -v: %v, want %v (must equal -a)", got, want)
	}
	if got := assemble(info, selection{version: true}); len(got) != 0 {
		t.Errorf("-v: %v, want no fields", got)
	}
}

// With a version present the Issue 7 order is sysname nodename release
// version machine, for -a and for -a -v equally.
func TestAssembleFixedOrderWithVersion(t *testing.T) {
	info := sysinfo{sysname: "S", nodename: "N", release: "R", version: "V", machine: "M"}
	want := []string{"S", "N", "R", "V", "M"}
	if got := assemble(info, selection{all: true}); !reflect.DeepEqual(got, want) {
		t.Errorf("-a: %v, want %v", got, want)
	}
	if got := assemble(info, selection{all: true, version: true}); !reflect.DeepEqual(got, want) {
		t.Errorf("-a -v: %v, want %v", got, want)
	}
	if got := assemble(info, selection{version: true}); !reflect.DeepEqual(got, []string{"V"}) {
		t.Errorf("-v: %v, want [V]", got)
	}
}
