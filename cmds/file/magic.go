package filecmd

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/qiangli/coreutils/tool"
)

type testPlan struct {
	position    []positionTests
	context     bool
	prefixBytes uint64
	ranges      []byteRange
}

const (
	// Context tests deliberately inspect a prefix, as historical file(1)
	// implementations do. Fixed position tests need at most byte 261, while
	// source-language and text tests benefit from a larger sample. The dynamic
	// PE signature offset is captured as its own sparse range.
	defaultPositionBytes = 262
	contextPrefixBytes   = 64 * 1024
)

type positionTests struct {
	defaults bool
	magic    []magicTest
}

type magicTest struct {
	offset       uint64
	continuation bool
	typeSpec     magicType
	comparison   byte
	want         uint64
	stringValue  []byte
	message      string
}

type magicType struct {
	stringValue bool
	signed      bool
	size        int
	mask        uint64
	hasMask     bool
}

func loadTestPlan(rc *tool.RunContext, sources []testSource) (testPlan, error) {
	hasMagic, hasReplacement, hasDefault := false, false, false
	for _, source := range sources {
		switch source.kind {
		case additionalMagic:
			hasMagic = true
		case replacementMagic:
			hasReplacement = true
		case defaultTests:
			hasDefault = true
		}
	}
	if !hasMagic && !hasReplacement && !hasDefault {
		sources = append(sources, testSource{kind: defaultTests})
	} else if hasMagic && !hasReplacement && !hasDefault {
		sources = append(sources, testSource{kind: defaultTests})
	}

	var plan testPlan
	for _, source := range sources {
		if source.kind == defaultTests {
			plan.position = append(plan.position, positionTests{defaults: true})
			plan.context = true
			continue
		}
		tests, err := loadMagicFile(rc, source.name)
		if err != nil {
			return testPlan{}, err
		}
		plan.position = append(plan.position, positionTests{magic: tests})
	}
	prefix, ranges, err := plan.inspectionRanges()
	if err != nil {
		return testPlan{}, err
	}
	plan.prefixBytes, plan.ranges = prefix, ranges
	return plan, nil
}

func (p testPlan) inspectionRanges() (uint64, []byteRange, error) {
	// One byte is needed even for an empty replacement plan so empty input can
	// be distinguished from non-empty data.
	prefix := uint64(1)
	if p.context {
		prefix = contextPrefixBytes
	}
	ranges := []byteRange{{end: prefix}}
	for _, group := range p.position {
		if group.defaults && prefix < defaultPositionBytes {
			prefix = defaultPositionBytes
			ranges[0].end = prefix
		}
		for _, test := range group.magic {
			width := test.typeSpec.size
			if test.typeSpec.stringValue {
				width = len(test.stringValue)
			}
			if test.offset > ^uint64(0)-uint64(width) {
				return 0, nil, fmt.Errorf("magic test offset overflows its value width")
			}
			end := test.offset + uint64(width)
			ranges = append(ranges, byteRange{start: test.offset, end: end})
		}
	}
	return prefix, mergeRanges(ranges), nil
}

func (p testPlan) classify(data []byte, utf8Locale bool) (string, error) {
	return p.classifyInspected(inspectedData{chunks: []dataChunk{{data: data}}}, utf8Locale)
}

func (p testPlan) classifyInspected(data inspectedData, utf8Locale bool) (string, error) {
	prefix := data.prefix()
	if len(prefix) == 0 {
		return "empty", nil
	}
	for _, tests := range p.position {
		if tests.defaults {
			if typ, ok := classifyPositionInspected(data); ok {
				return typ, nil
			}
			continue
		}
		message, ok, err := applyMagicInspected(data, tests.magic)
		if err != nil {
			return "", err
		}
		if ok {
			return message, nil
		}
	}
	if p.context {
		return classifyContext(prefix, utf8Locale), nil
	}
	return "data", nil
}

func loadMagicFile(rc *tool.RunContext, name string) ([]magicTest, error) {
	data, err := os.ReadFile(rc.Path(name))
	if err != nil {
		return nil, fmt.Errorf("%s: %v", name, tool.SysErr(err))
	}
	var tests []magicTest
	for lineNo, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		test, err := parseMagicLine(line)
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %v", name, lineNo+1, err)
		}
		if test.continuation && !hasPrimary(tests) {
			return nil, fmt.Errorf("%s:%d: continuation has no preceding primary test", name, lineNo+1)
		}
		tests = append(tests, test)
	}
	return tests, nil
}

func hasPrimary(tests []magicTest) bool {
	for i := len(tests) - 1; i >= 0; i-- {
		if !tests[i].continuation {
			return true
		}
	}
	return false
}

func parseMagicLine(line string) (magicTest, error) {
	offsetField, typeField, valueField, message, ok := splitMagicFields(line)
	if !ok {
		return magicTest{}, fmt.Errorf("expected four fields")
	}

	test := magicTest{message: message, comparison: '='}
	if strings.HasPrefix(offsetField, ">") {
		test.continuation = true
		offsetField = offsetField[1:]
	}
	if offsetField == "" || strings.HasPrefix(offsetField, ">") {
		return magicTest{}, fmt.Errorf("invalid magic offset %q", offsetField)
	}
	if !isUnsignedMagicNumber(offsetField) {
		return magicTest{}, fmt.Errorf("invalid magic offset %q", offsetField)
	}
	offset, err := strconv.ParseUint(offsetField, 0, 64)
	if err != nil {
		return magicTest{}, fmt.Errorf("invalid magic offset %q", offsetField)
	}
	test.offset = offset
	test.typeSpec, err = parseMagicType(typeField)
	if err != nil {
		return magicTest{}, err
	}
	if test.typeSpec.stringValue {
		test.stringValue, err = unescapeMagicString(valueField)
		if err != nil {
			return magicTest{}, err
		}
	} else if valueField == "x" {
		test.comparison = 'x'
	} else {
		if len(valueField) > 0 && strings.ContainsRune("=<>&^", rune(valueField[0])) {
			test.comparison = valueField[0]
			valueField = valueField[1:]
		}
		test.want, err = parseMagicNumber(valueField)
		if err != nil {
			return magicTest{}, fmt.Errorf("invalid magic value %q", valueField)
		}
	}
	if _, err := renderMagicMessage(test.message, test.messageArg(0)); err != nil {
		return magicTest{}, err
	}
	return test, nil
}

func splitMagicFields(line string) (offset, typ, value, message string, ok bool) {
	// Tabs are the normative separators. Split exactly three so leading
	// whitespace and additional tabs in the message remain message data.
	if strings.Count(line, "\t") >= 3 {
		fields := strings.SplitN(line, "\t", 4)
		return strings.TrimSpace(fields[0]), strings.TrimSpace(fields[1]), strings.TrimSpace(fields[2]), fields[3], true
	}
	offset, rest, ok := magicField(line)
	if !ok {
		return "", "", "", "", false
	}
	typ, rest, ok = magicField(rest)
	if !ok {
		return "", "", "", "", false
	}
	value, message, ok = magicField(rest)
	if !ok {
		return "", "", "", "", false
	}
	return offset, typ, value, strings.TrimLeftFunc(message, unicode.IsSpace), true
}

// magicField consumes one whitespace-delimited field. A backslash quotes the
// following byte for field-splitting purposes; unescaping happens later.
func magicField(input string) (field, rest string, ok bool) {
	input = strings.TrimLeftFunc(input, unicode.IsSpace)
	if input == "" {
		return "", "", false
	}
	escaped := false
	for i, r := range input {
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if unicode.IsSpace(r) {
			return input[:i], input[i:], true
		}
	}
	return input, "", false
}

func parseMagicType(field string) (magicType, error) {
	switch {
	case field == "s" || field == "string":
		return magicType{stringValue: true}, nil
	case field == "byte" || strings.HasPrefix(field, "byte&"):
		field = "dC" + field[len("byte"):]
	case field == "short" || strings.HasPrefix(field, "short&"):
		field = "dS" + field[len("short"):]
	case field == "long" || strings.HasPrefix(field, "long&"):
		field = "dL" + field[len("long"):]
	}
	if field == "" || (field[0] != 'd' && field[0] != 'u') {
		return magicType{}, fmt.Errorf("unsupported magic type %q", field)
	}
	t := magicType{signed: field[0] == 'd', mask: ^uint64(0)}
	rest := field[1:]
	if before, after, found := strings.Cut(rest, "&"); found {
		rest = before
		if !isUnsignedMagicNumber(after) {
			return magicType{}, fmt.Errorf("invalid magic mask %q", after)
		}
		mask, err := strconv.ParseUint(after, 0, 64)
		if err != nil {
			return magicType{}, fmt.Errorf("invalid magic mask %q", after)
		}
		t.mask, t.hasMask = mask, true
	}
	switch rest {
	case "":
		// POSIX specifies the bare d and u forms in terms of the C int
		// type, not the host word size. Go's int is 64 bits on most of our
		// targets, whereas the C ABI's int is 32 bits on every supported Go
		// target. Keep this independent of the Go word size so a portable
		// magic entry such as "0 d 0x04030201" reads four bytes everywhere.
		t.size = cIntSize()
	case "C":
		t.size = 1
	case "S":
		t.size = 2
	case "I":
		t.size = 4
	case "L":
		t.size = cLongSize()
	default:
		if !isDecimalDigits(rest) {
			return magicType{}, fmt.Errorf("unsupported magic type width %q", rest)
		}
		size, err := strconv.Atoi(rest)
		if err != nil || size < 1 || size > 8 {
			return magicType{}, fmt.Errorf("unsupported magic type width %q", rest)
		}
		t.size = size
	}
	return t, nil
}

func isDecimalDigits(value string) bool {
	if value == "" {
		return false
	}
	for i := range len(value) {
		if value[i] < '0' || value[i] > '9' {
			return false
		}
	}
	return true
}

func isUnsignedMagicNumber(value string) bool {
	digits := value
	base := byte(10)
	if strings.HasPrefix(digits, "0x") || strings.HasPrefix(digits, "0X") {
		digits, base = digits[2:], 16
	} else if len(digits) > 1 && digits[0] == '0' {
		base = 8
	}
	if digits == "" {
		return false
	}
	for i := range len(digits) {
		c := digits[i]
		if c >= '0' && c <= '7' {
			continue
		}
		if base == 10 && (c == '8' || c == '9') {
			continue
		}
		if base == 16 && ((c >= '8' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			continue
		}
		return false
	}
	return true
}

func cIntSize() int { return 4 }

func cLongSize() int {
	if runtime.GOOS == "windows" {
		return 4
	}
	return strconv.IntSize / 8
}

func parseMagicNumber(value string) (uint64, error) {
	if strings.HasPrefix(value, "-") || strings.HasPrefix(value, "+") {
		n, err := strconv.ParseInt(value, 10, 64)
		return uint64(n), err
	}
	if strings.HasPrefix(value, "0x") || strings.HasPrefix(value, "0X") || (len(value) > 1 && value[0] == '0') {
		return strconv.ParseUint(value, 0, 64)
	}
	n, err := strconv.ParseInt(value, 10, 64)
	return uint64(n), err
}

func unescapeMagicString(value string) ([]byte, error) {
	out := make([]byte, 0, len(value))
	for i := 0; i < len(value); i++ {
		if value[i] != '\\' {
			out = append(out, value[i])
			continue
		}
		i++
		if i == len(value) {
			return nil, fmt.Errorf("trailing backslash in magic string")
		}
		if value[i] >= '0' && value[i] <= '7' {
			end := i + 1
			for end < len(value) && end < i+3 && value[end] >= '0' && value[end] <= '7' {
				end++
			}
			n, _ := strconv.ParseUint(value[i:end], 8, 8)
			out = append(out, byte(n))
			i = end - 1
			continue
		}
		escapes := map[byte]byte{'\\': '\\', 'a': '\a', 'b': '\b', 'f': '\f', 'n': '\n', 'r': '\r', 't': '\t', 'v': '\v', ' ': ' '}
		b, ok := escapes[value[i]]
		if !ok {
			return nil, fmt.Errorf("unsupported magic escape \\%c", value[i])
		}
		out = append(out, b)
	}
	return out, nil
}

func applyMagic(data []byte, tests []magicTest) (string, bool, error) {
	return applyMagicInspected(inspectedData{chunks: []dataChunk{{data: data}}}, tests)
}

func applyMagicInspected(data inspectedData, tests []magicTest) (string, bool, error) {
	for i := 0; i < len(tests); {
		primary := tests[i]
		i++
		if primary.continuation {
			continue
		}
		value, matched := primary.match(data)
		if !matched {
			for i < len(tests) && tests[i].continuation {
				i++
			}
			continue
		}
		message, err := renderMagicMessage(primary.message, primary.messageArg(value))
		if err != nil {
			return "", false, err
		}
		for i < len(tests) && tests[i].continuation {
			child := tests[i]
			i++
			if childValue, ok := child.match(data); ok {
				part, err := renderMagicMessage(child.message, child.messageArg(childValue))
				if err != nil {
					return "", false, err
				}
				message += part
			}
		}
		return message, true, nil
	}
	return "", false, nil
}

func (t magicTest) match(data inspectedData) (uint64, bool) {
	if t.typeSpec.stringValue {
		value, ok := data.bytesAt(t.offset, len(t.stringValue))
		return 0, ok && bytes.Equal(value, t.stringValue)
	}
	valueBytes, ok := data.bytesAt(t.offset, t.typeSpec.size)
	if !ok {
		return 0, false
	}
	value := nativeUint(valueBytes)
	if t.typeSpec.hasMask {
		value &= t.typeSpec.mask
	}
	if t.comparison == 'x' {
		return value, true
	}
	want := t.want & widthMask(t.typeSpec.size)
	switch t.comparison {
	case '=':
		return value, value == want
	case '<':
		if t.typeSpec.signed {
			return value, signExtend(value, t.typeSpec.size) < signExtend(want, t.typeSpec.size)
		}
		return value, value < want
	case '>':
		if t.typeSpec.signed {
			return value, signExtend(value, t.typeSpec.size) > signExtend(want, t.typeSpec.size)
		}
		return value, value > want
	case '&':
		return value, value&want == want
	case '^':
		return value, value&want != want
	default:
		return value, false
	}
}

func nativeUint(data []byte) uint64 {
	var padded [8]byte
	if binary.NativeEndian.Uint16([]byte{1, 0}) == 1 {
		copy(padded[:], data)
	} else {
		copy(padded[8-len(data):], data)
	}
	return binary.NativeEndian.Uint64(padded[:])
}

func widthMask(size int) uint64 {
	if size == 8 {
		return ^uint64(0)
	}
	return uint64(1)<<(size*8) - 1
}

func signExtend(value uint64, size int) int64 {
	shift := 64 - size*8
	return int64(value<<shift) >> shift
}

type magicNumberArg struct {
	value  uint64
	size   int
	signed bool
}

func (t magicTest) messageArg(value uint64) any {
	if t.typeSpec.stringValue {
		return string(t.stringValue)
	}
	return magicNumberArg{value: value, size: t.typeSpec.size, signed: t.typeSpec.signed}
}

func renderMagicMessage(message string, arg any) (string, error) {
	var out strings.Builder
	usedArgument := false
	for i := 0; i < len(message); {
		if message[i] == '\\' {
			value, consumed, err := magicFormatEscape(message[i:])
			if err != nil {
				return "", err
			}
			out.WriteByte(value)
			i += consumed
			continue
		}
		if message[i] != '%' {
			out.WriteByte(message[i])
			i++
			continue
		}
		if i+1 < len(message) && message[i+1] == '%' {
			out.WriteByte('%')
			i += 2
			continue
		}
		start := i
		i++
		for i < len(message) && strings.ContainsRune("#0- +", rune(message[i])) {
			i++
		}
		for i < len(message) && message[i] >= '0' && message[i] <= '9' {
			i++
		}
		if i < len(message) && message[i] == '.' {
			i++
			for i < len(message) && message[i] >= '0' && message[i] <= '9' {
				i++
			}
		}
		for i < len(message) && strings.ContainsRune("hljztL", rune(message[i])) {
			i++
		}
		if i >= len(message) || !strings.ContainsRune("bcdiouxXs", rune(message[i])) {
			return "", fmt.Errorf("unsupported magic message format near %q", message[start:])
		}
		conv := message[i]
		spec := message[start : i+1]
		for _, modifier := range []string{"h", "l", "j", "z", "t", "L"} {
			spec = strings.ReplaceAll(spec, modifier, "")
		}
		if conv == 'i' || conv == 'u' {
			spec = spec[:len(spec)-1] + "d"
		}
		formatArg := arg
		if usedArgument {
			if conv == 'c' {
				formatArg = rune(0)
			} else if strings.ContainsRune("bs", rune(conv)) {
				formatArg = ""
			} else {
				formatArg = uint64(0)
			}
		} else if number, ok := arg.(magicNumberArg); ok {
			switch conv {
			case 'd', 'i':
				if number.signed {
					formatArg = signExtend(number.value, number.size)
				} else {
					formatArg = number.value
				}
			case 'c':
				formatArg = rune(number.value)
			default:
				formatArg = number.value
			}
		} else if text, ok := arg.(string); ok && conv == 'c' {
			formatArg = rune(0)
			if first, _ := utf8.DecodeRuneInString(text); len(text) != 0 {
				formatArg = first
			}
		}
		if conv == 'b' {
			value, halt, err := magicBString(formatArg)
			if err != nil {
				return "", err
			}
			spec = spec[:len(spec)-1] + "s"
			out.WriteString(fmt.Sprintf(spec, value))
			if halt {
				return out.String(), nil
			}
			usedArgument = true
			i++
			continue
		}
		out.WriteString(fmt.Sprintf(spec, formatArg))
		usedArgument = true
		i++
	}
	return out.String(), nil
}

func magicBString(arg any) (string, bool, error) {
	var input string
	switch value := arg.(type) {
	case string:
		input = value
	case magicNumberArg:
		if value.signed {
			input = strconv.FormatInt(signExtend(value.value, value.size), 10)
		} else {
			input = strconv.FormatUint(value.value, 10)
		}
	default:
		input = fmt.Sprint(value)
	}
	var out strings.Builder
	for i := 0; i < len(input); {
		if input[i] != '\\' {
			out.WriteByte(input[i])
			i++
			continue
		}
		if i+1 >= len(input) {
			return "", false, fmt.Errorf("trailing backslash in %%b magic argument")
		}
		if input[i+1] == 'c' {
			return out.String(), true, nil
		}
		if input[i+1] == '0' {
			end := i + 2
			for end < len(input) && end < i+5 && input[end] >= '0' && input[end] <= '7' {
				end++
			}
			n, _ := strconv.ParseUint(input[i+2:end], 8, 8)
			out.WriteByte(byte(n))
			i = end
			continue
		}
		value, consumed, err := magicFormatEscape(input[i:])
		if err != nil {
			return "", false, err
		}
		out.WriteByte(value)
		i += consumed
	}
	return out.String(), false, nil
}

func magicFormatEscape(value string) (byte, int, error) {
	if len(value) < 2 {
		return 0, 0, fmt.Errorf("trailing backslash in magic message")
	}
	if value[1] >= '0' && value[1] <= '7' {
		end := 2
		for end < len(value) && end < 4 && value[end] >= '0' && value[end] <= '7' {
			end++
		}
		n, _ := strconv.ParseUint(value[1:end], 8, 8)
		return byte(n), end, nil
	}
	escapes := map[byte]byte{'\\': '\\', 'a': '\a', 'b': '\b', 'f': '\f', 'n': '\n', 'r': '\r', 't': '\t', 'v': '\v'}
	b, ok := escapes[value[1]]
	if !ok {
		return 0, 0, fmt.Errorf("unsupported magic message escape \\%c", value[1])
	}
	return b, 2, nil
}
