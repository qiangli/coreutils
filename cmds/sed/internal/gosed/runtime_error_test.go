package gosed

import (
	"errors"
	"reflect"
	"testing"
)

type errorRegexp struct{ err error }

func (r *errorRegexp) MatchString(string) (bool, error) { return false, r.err }
func (r *errorRegexp) FindAllStringSubmatchIndex(string, int) ([][]int, error) {
	return nil, r.err
}
func (r *errorRegexp) FindAllSubmatchIndex([]byte, int) ([][]int, error) {
	return nil, r.err
}
func (*errorRegexp) ExpandString([]byte, string, string, []int) ([]byte, error) {
	panic("ExpandString called after matcher error")
}
func (*errorRegexp) Expand([]byte, []byte, []byte, []int) ([]byte, error) {
	panic("Expand called after matcher error")
}

type expansionErrorRegexp struct{ err error }

func (*expansionErrorRegexp) MatchString(string) (bool, error) { return true, nil }
func (*expansionErrorRegexp) FindAllStringSubmatchIndex(s string, _ int) ([][]int, error) {
	return [][]int{{0, len(s)}}, nil
}
func (*expansionErrorRegexp) FindAllSubmatchIndex(s []byte, _ int) ([][]int, error) {
	return [][]int{{0, len(s)}}, nil
}
func (r *expansionErrorRegexp) ExpandString(dst []byte, _ string, _ string, _ []int) ([]byte, error) {
	return append(dst, "partial"...), r.err
}
func (r *expansionErrorRegexp) Expand(dst, _ []byte, _ []byte, _ []int) ([]byte, error) {
	return append(dst, "partial"...), r.err
}

func TestRuntimeRegexpErrorsDoNotMutateInstructionState(t *testing.T) {
	wantErr := errors.New("matcher failed")
	failing := &errorRegexp{err: wantErr}
	prior := &errorRegexp{err: errors.New("prior")}

	t.Run("single address", func(t *testing.T) {
		svm := &vm{pat: "subject", ip: 7, lastRE: prior}
		cmd := &cmd_simplecond{cond: &regexpcond{re: failing}, metloc: 1, unmetloc: 2}
		if err := cmd.run(svm); !errors.Is(err, wantErr) {
			t.Fatalf("run error = %v, want %v", err, wantErr)
		}
		if svm.ip != 7 || svm.lastRE != prior {
			t.Fatalf("matcher error changed VM state: ip=%d lastRE=%p", svm.ip, svm.lastRE)
		}
	})

	t.Run("two address start and stale range", func(t *testing.T) {
		svm := &vm{pat: "subject", lineno: 4, ip: 7, lastRE: prior}
		cmd := &cmd_twocond{
			start: &regexpcond{re: failing}, end: numbercond(9), metloc: 1, unmetloc: 2,
			isOn: true, offFrom: 2,
		}
		if err := cmd.run(svm); !errors.Is(err, wantErr) {
			t.Fatalf("run error = %v, want %v", err, wantErr)
		}
		if svm.ip != 7 || svm.lastRE != prior || !cmd.isOn || cmd.offFrom != 2 {
			t.Fatalf("matcher error changed range/VM state: ip=%d lastRE=%p isOn=%v offFrom=%d", svm.ip, svm.lastRE, cmd.isOn, cmd.offFrom)
		}
	})

	t.Run("two address end", func(t *testing.T) {
		svm := &vm{pat: "subject", lineno: 4, ip: 7, lastRE: prior}
		cmd := &cmd_twocond{
			start: numbercond(1), end: &regexpcond{re: failing}, metloc: 1, unmetloc: 2,
			isOn: true,
		}
		if err := cmd.run(svm); !errors.Is(err, wantErr) {
			t.Fatalf("run error = %v, want %v", err, wantErr)
		}
		if svm.ip != 7 || svm.lastRE != prior || !cmd.isOn || cmd.offFrom != 0 {
			t.Fatalf("matcher error changed range/VM state: ip=%d lastRE=%p isOn=%v offFrom=%d", svm.ip, svm.lastRE, cmd.isOn, cmd.offFrom)
		}
	})

	for _, tc := range []struct {
		name   string
		null   bool
		lastRE sedRegexp
	}{
		{name: "full substitution", lastRE: prior},
		{name: "null fallback", null: true},
		{name: "null reuse", null: true, lastRE: failing},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pattern := failing
			if tc.name == "null reuse" {
				pattern = &errorRegexp{err: errors.New("unused fallback")}
			}
			output := []byte("guard")
			svm := &vm{
				pat: "subject", hold: "hold", ip: 7, modified: false,
				lastRE: tc.lastRE, output: output,
			}
			beforeOutput := append([]byte(nil), output...)
			cmd := &substitute{
				pattern: pattern, replacement: "changed", null: tc.null,
				pflag: true, wfile: "must-not-write",
				writeFile: func(string, string) error {
					panic("write file called after matcher error")
				},
			}
			if err := cmd.run(svm); !errors.Is(err, wantErr) {
				t.Fatalf("run error = %v, want %v", err, wantErr)
			}
			if svm.ip != 7 || svm.pat != "subject" || svm.modified || svm.lastRE != tc.lastRE || !reflect.DeepEqual(output, beforeOutput) {
				t.Fatalf("matcher error changed substitution state: ip=%d pat=%q modified=%v lastRE=%p output=%q", svm.ip, svm.pat, svm.modified, svm.lastRE, output)
			}
		})
	}

	t.Run("engine surfaces substitution error", func(t *testing.T) {
		cmd := &substitute{pattern: failing, replacement: "changed"}
		engine := &Engine{ins: []instruction{cmd_fillNext, cmd.run}}
		out, err := engine.RunString("subject\n")
		if !errors.Is(err, wantErr) {
			t.Fatalf("RunString error = %v, want %v", err, wantErr)
		}
		if out != "" {
			t.Fatalf("RunString output = %q after matcher error, want empty", out)
		}
	})

	for _, tc := range []struct {
		name   string
		null   bool
		lastRE sedRegexp
	}{
		{name: "full substitution expansion", lastRE: prior},
		{name: "null fallback expansion", null: true},
		{name: "null reuse expansion", null: true, lastRE: &expansionErrorRegexp{err: wantErr}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pattern := sedRegexp(&expansionErrorRegexp{err: wantErr})
			if tc.name == "null reuse expansion" {
				pattern = prior
			}
			output := []byte("guard")
			svm := &vm{pat: "subject", ip: 7, lastRE: tc.lastRE, output: output}
			beforeOutput := append([]byte(nil), output...)
			cmd := &substitute{
				pattern: pattern, replacement: "changed", null: tc.null,
				pflag: true, wfile: "must-not-write",
				writeFile: func(string, string) error {
					panic("write file called after expansion error")
				},
			}
			if err := cmd.run(svm); !errors.Is(err, wantErr) {
				t.Fatalf("run error = %v, want %v", err, wantErr)
			}
			if svm.ip != 7 || svm.pat != "subject" || svm.modified || svm.lastRE != tc.lastRE || !reflect.DeepEqual(output, beforeOutput) {
				t.Fatalf("expansion error changed substitution state: ip=%d pat=%q modified=%v lastRE=%p output=%q", svm.ip, svm.pat, svm.modified, svm.lastRE, output)
			}
		})
	}
}
