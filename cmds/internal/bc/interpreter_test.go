package bc

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

func execute(t *testing.T, src string, mathlib bool) (string, error) {
	t.Helper()
	var out bytes.Buffer
	b := New(&out, strings.NewReader(""))
	b.Mathlib = mathlib
	if mathlib {
		b.Scale = 20
	}
	err := b.Execute(src)
	return out.String(), err
}

func TestPOSIXStatementsAndUnbracedControl(t *testing.T) {
	src := `i=0
if(i==0) i=1
while(i<3) i++
for(j=0;j<3;j++) j
i
{ 8; 9 }
`
	got, err := execute(t, src, false)
	if err != nil {
		t.Fatal(err)
	}
	if want := "1\n2\n0\n1\n2\n3\n8\n9\n"; got != want {
		t.Fatalf("stdout=%q, want %q", got, want)
	}
}

func TestPOSIXQuitIsLexical(t *testing.T) {
	for _, src := range []string{
		"1\nquit\n@\n",
		"1\nif(0) quit\n2\n",
		"1\ndefine f() {\nquit @ this syntax is never read",
	} {
		got, err := execute(t, src, false)
		if !errors.Is(err, io.EOF) || got != "1\n" {
			t.Fatalf("src=%q: stdout=%q err=%v", src, got, err)
		}
	}
}

func TestPOSIXStreamingBeforeError(t *testing.T) {
	got, err := execute(t, "1\n2\n@\n", false)
	if err == nil || got != "1\n2\n" {
		t.Fatalf("stdout=%q err=%v", got, err)
	}
}

func TestPOSIXLexicalConventionsAndLimits(t *testing.T) {
	src := "12\\\n34\n1/* comment */+2\nibase=16\nAB12\nibase=A\n\"a\\\n b\"\n"
	got, err := execute(t, src, false)
	if err != nil {
		t.Fatal(err)
	}
	if want := "1234\n3\n43794\na\\\n b"; got != want {
		t.Fatalf("stdout=%q, want %q", got, want)
	}
	_, err = execute(t, `"`+strings.Repeat("x", 1001)+`"`, false)
	if err == nil || !strings.Contains(err.Error(), "BC_STRING_MAX") {
		t.Fatalf("oversize string err=%v", err)
	}
}

func TestPOSIXRegisterAssignmentAndBases(t *testing.T) {
	src := `scale=1.9
scale
ibase=8
ibase=A
ibase
obase=99.9
1000
`
	got, err := execute(t, src, false)
	if err != nil {
		t.Fatal(err)
	}
	if want := "1\n10\n 10 10\n"; got != want {
		t.Fatalf("stdout=%q, want %q", got, want)
	}
	tests := []struct {
		src  string
		want string
		err  error
	}{
		{`if(1) 3`, "3\n", nil},
		{`for(;;) { 4; break }`, "4\n", nil},
		{`define f() { return (); }; f()`, "0\n", nil},
	}
	for _, tc := range tests {
		got, err := execute(t, tc.src, false)
		if tc.err == nil && err != nil {
			t.Fatalf("src=%q err=%v", tc.src, err)
		}
		if got != tc.want {
			t.Fatalf("src=%q stdout=%q want %q", tc.src, got, tc.want)
		}
	}
	for _, src := range []string{"scale=100\n", "ibase=1\n", "obase=1000\n"} {
		if _, err := execute(t, src, false); err == nil {
			t.Fatalf("expected range error for %q", src)
		}
	}
}

func TestPOSIXParenthesizedAssignmentIsPrinted(t *testing.T) {
	got, err := execute(t, "a=4\n(a=8)\na\nibase=2\nobase=(A)\nobase\n", false)
	if err != nil {
		t.Fatal(err)
	}
	if want := "8\n8\n10\n"; got != want {
		t.Fatalf("stdout=%q, want %q", got, want)
	}
}

func TestReferenceOutputBasesAbovePOSIXMinimum(t *testing.T) {
	got, err := execute(t, "obase=100\n257\n257.95\nobase=101\n1111111111111111\n1234567890123456\n105101005\nobase=999\n999\n998\nobase=10\n10\n", false)
	if err != nil {
		t.Fatal(err)
	}
	want := " 02 57\n 02 57.95\n" +
		" 010 036 072 041 038 070 044 000\n" +
		" 011 052 001 090 077 010 026 089\n" +
		" 001 001 001 001 001\n" +
		" 001 000\n 998\n10\n"
	if got != want {
		t.Fatalf("stdout=%q, want %q", got, want)
	}
}

func TestOutputBaseBelowMinimumIsConstrainedAndExecutionContinues(t *testing.T) {
	got, err := execute(t, "obase=0\n0\nobase\nobase=1.9\n3\n", false)
	if err != nil {
		t.Fatal(err)
	}
	if want := "0\n10\n11\n"; got != want {
		t.Fatalf("stdout=%q, want %q", got, want)
	}
}

func TestPOSIXArithmeticScaleTable(t *testing.T) {
	src := `scale=3
scale(1.20+3.400)
scale(3.400-1.20)
scale(1.20*3.400)
scale(1.20/3.400)
scale(5.50%2.0)
scale(1.20^3)
scale(1.20^-3)
2^2.0
x=1.20
scale(++x)
scale(x--)
`
	got, err := execute(t, src, false)
	if err != nil {
		t.Fatal(err)
	}
	if want := "3\n3\n3\n3\n4\n3\n3\n4\n2\n2\n"; got != want {
		t.Fatalf("stdout=%q, want %q", got, want)
	}
}

func TestPOSIXBuiltins(t *testing.T) {
	src := `length(123.40)
length(.00100)
length(0.000)
scale(1.230)
scale=4
sqrt(2)
scale(sqrt(2.00))
`
	got, err := execute(t, src, false)
	if err != nil {
		t.Fatal(err)
	}
	if want := "5\n3\n3\n3\n1.4142\n4\n"; got != want {
		t.Fatalf("stdout=%q, want %q", got, want)
	}
}

func TestPOSIXArraysCopyByValueAndIndexLimits(t *testing.T) {
	src := `define f(a[],x) {
auto z[]
a[0]=99
z[1]=x
return(a[1]+z[1])
}
b[0]=1
b[1.9]=4
f(b[],3)
b[0]
b[1]
`
	got, err := execute(t, src, false)
	if err != nil {
		t.Fatal(err)
	}
	if want := "7\n1\n4\n"; got != want {
		t.Fatalf("stdout=%q, want %q", got, want)
	}
	if _, err := execute(t, "a[2048]=1\n", false); err == nil {
		t.Fatal("array index 2048 unexpectedly accepted")
	}
}

func TestPOSIXFunctionsArraysAndDynamicLocals(t *testing.T) {
	src := `x=4
define g(x) {
return(x+1)
}
define f(x,y) {
auto z
z=g(x)
return(z+y)
}
f(1,x)
x
ibase=16
define h() {
return(10)
}
ibase=A
h()
`
	got, err := execute(t, src, false)
	if err != nil {
		t.Fatal(err)
	}
	if want := "6\n4\n10\n"; got != want {
		t.Fatalf("stdout=%q, want %q", got, want)
	}
}

func TestPOSIXOutputBasesAndLineFolding(t *testing.T) {
	src := `obase=2
scale=3
1/3
obase=20
25
obase=10
10^80
`
	got, err := execute(t, src, false)
	if err != nil {
		t.Fatal(err)
	}
	want := ".0101010100\n 01 05\n" + strings.Repeat("0", 68) + "\\\n" + strings.Repeat("0", 13) + "\n"
	// 10^80 is one followed by 80 zeroes.
	want = strings.Replace(want, strings.Repeat("0", 68), "1"+strings.Repeat("0", 67), 1)
	if got != want {
		t.Fatalf("stdout=%q, want %q", got, want)
	}
}

func TestPOSIXMathLibraryAllFunctions(t *testing.T) {
	src := `scale=5
s(0)
c(0)
a(1)
l(e(1))
j(0,0)
j(1,0)
j(0,1)
scale(j(0,1))
scale
`
	got, err := execute(t, src, true)
	if err != nil {
		t.Fatal(err)
	}
	if want := "0\n1.00000\n.78539\n.99999\n1.00000\n0\n.76519\n5\n5\n"; got != want {
		t.Fatalf("stdout=%q, want %q", got, want)
	}
}

func TestPOSIXMathFunctionCanBeRedefined(t *testing.T) {
	got, err := execute(t, "define s(x) {\nreturn(42)\n}\ns(0)\n", true)
	if err != nil || got != "42\n" {
		t.Fatalf("stdout=%q err=%v", got, err)
	}
}
