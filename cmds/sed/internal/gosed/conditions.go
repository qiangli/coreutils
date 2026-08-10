package gosed

import "fmt"

// conditions are what I'm calling the '1,10' in
// commands ike '1,10 d'.  They are the line numbers,
// regexps, and '$' that you can use to control when
// commands execute.

type condition interface {
	isMet(svm *vm) bool
}

// -----------------------------------------------------
type numbercond int // for matching line number conditions

func (n numbercond) isMet(svm *vm) bool {
	return svm.lineno == int(n)
}

// relativecond implements GNU's second-address form addr,+N. Its target is
// reset whenever the first address begins a new range.
type relativecond struct {
	lines  int
	target int
}

func (r *relativecond) startRange(svm *vm) bool {
	r.target = svm.lineno + r.lines
	return r.lines == 0
}

func (r *relativecond) isMet(svm *vm) bool {
	return svm.lineno >= r.target
}

// -----------------------------------------------------
type eofcond struct{} // for matching the condition '$'

func (_ eofcond) isMet(svm *vm) bool {
	return svm.lastl
}

// -----------------------------------------------------
type regexpcond struct {
	re   sedRegexp // for matching regexp conditions
	null bool      // written as //: resolve against the last RE at run time
}

func (r *regexpcond) isMet(svm *vm) (answer bool) {
	re := resolveNullRE(svm, r.re, r.null)
	return re.MatchString(svm.pat)
}

// newRECondition compiles a context address. An empty RE is POSIX's null RE:
// it stands for the last RE used, so it compiles against last — the most
// recent RE lexically before it — while remembering to prefer whatever RE the
// program has most recently APPLIED once it is running (see resolveNullRE).
func newRECondition(opts Options, s string, last string, loc *location) (*regexpcond, error) {
	null := s == ""
	if null {
		if last == "" {
			return nil, fmt.Errorf("no previous regular expression %v", loc)
		}
		s = last
	}

	re, err := opts.compileRE(s, "") // GNU BRE/ERE → RE2 (was regexp.Compile)
	if err != nil {
		err = fmt.Errorf("Regexp Error: %s %v", err.Error(), loc)
	}
	return &regexpcond{re, null}, err
}

// resolveNullRE implements the null-RE rule for one use of one RE: "the last
// RE used" is dynamic, so a // address or s//repl/ takes the RE the running
// program applied most recently, not the one that lexically precedes it. The
// compiled fallback covers the case where nothing has been applied yet — a
// null RE reachable before its predecessor ever runs, e.g. behind a branch.
//
// Every non-null use records itself, which is what makes the rule dynamic.
func resolveNullRE(svm *vm, re sedRegexp, null bool) sedRegexp {
	if null && svm.lastRE != nil {
		re = svm.lastRE
	}
	svm.lastRE = re
	return re
}
