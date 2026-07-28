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
	re sedRegexp // for matching regexp conditions
}

func (r *regexpcond) isMet(svm *vm) (answer bool) {
	return r.re.MatchString(svm.pat)
}

func newRECondition(s string, loc *location) (*regexpcond, error) {
	re, err := compileRE(s, "") // GNU BRE/ERE → RE2 (was regexp.Compile)
	if err != nil {
		err = fmt.Errorf("Regexp Error: %s %v", err.Error(), loc)
	}
	return &regexpcond{re}, err
}
