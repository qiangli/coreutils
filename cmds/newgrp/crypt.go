package newgrpcmd

import (
	"crypto/md5"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/subtle"
	"fmt"
	"hash"
	"strconv"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// This file verifies a crypt(3) password hash. It is written from the published
// algorithm specifications (Ulrich Drepper's SHA-crypt specification for $5$
// and $6$; the historical MD5-crypt description for $1$) and validated against
// an independent implementation's output — the vectors in crypt_test.go were
// produced with `openssl passwd -1/-5/-6`. No GNU or shadow-suite source was
// consulted or ported.
//
// The hard rule is at the bottom of verifyCrypt: a scheme this file does not
// implement is an ERROR, never a mismatch. Reporting "wrong password" for a
// yescrypt hash would send an operator hunting for a typo that does not exist,
// and reporting "correct" would be a security hole; only "I cannot check this"
// is true.

// cryptAlphabet is the crypt(3) base64 alphabet — NOT RFC 4648's. The digits
// come after the two punctuation characters, and each 6-bit group is emitted
// least-significant first.
const cryptAlphabet = "./0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// verifyCrypt reports whether plain hashes to want.
//
// An error means the hash could not be evaluated at all (unknown scheme,
// malformed field). Callers must treat that as "cannot authenticate", which is
// distinct from a false return meaning "authenticated and wrong".
func verifyCrypt(want, plain string) (bool, error) {
	switch {
	case strings.HasPrefix(want, "$1$"):
		got, err := md5Crypt(plain, want)
		if err != nil {
			return false, err
		}
		return constantTimeEqual(got, want), nil

	case strings.HasPrefix(want, "$5$"), strings.HasPrefix(want, "$6$"):
		got, err := shaCrypt(plain, want)
		if err != nil {
			return false, err
		}
		return constantTimeEqual(got, want), nil

	case strings.HasPrefix(want, "$2a$"), strings.HasPrefix(want, "$2b$"), strings.HasPrefix(want, "$2y$"):
		err := bcrypt.CompareHashAndPassword([]byte(want), []byte(plain))
		if err == nil {
			return true, nil
		}
		if err == bcrypt.ErrMismatchedHashAndPassword {
			return false, nil
		}
		return false, fmt.Errorf("malformed bcrypt hash: %w", err)

	default:
		// Everything else — yescrypt ($y$), gost-yescrypt ($gy$), scrypt ($7$),
		// NT ($3$), and the 13-character DES form — is unimplemented. Say so.
		return false, fmt.Errorf("unsupported password hash scheme %q", schemeOf(want))
	}
}

// schemeOf names the hash scheme for a diagnostic without echoing the hash
// itself: the salt and digest of a group password are not something to print.
func schemeOf(hash string) string {
	if !strings.HasPrefix(hash, "$") {
		return "descrypt"
	}
	if i := strings.Index(hash[1:], "$"); i >= 0 {
		return "$" + hash[1:1+i] + "$"
	}
	return "unknown"
}

func constantTimeEqual(a, b string) bool {
	// Length is not secret (the scheme fixes it), so a length check first is
	// fine; the byte comparison itself stays constant time.
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// --- SHA-crypt ($5$ / $6$) ---------------------------------------------------

const (
	shaDefaultRounds = 5000
	shaMinRounds     = 1000
	shaMaxRounds     = 999999999
	shaMaxSaltLen    = 16
)

// shaCrypt implements the SHA-256 and SHA-512 crypt schemes. setting supplies
// the scheme prefix, the optional rounds= parameter, and the salt; anything
// after the salt (an existing digest) is ignored, so a full hash can be passed
// straight back in for verification.
func shaCrypt(plain, setting string) (string, error) {
	var (
		newHash func() hash.Hash
		size    int
		prefix  string
	)
	switch {
	case strings.HasPrefix(setting, "$5$"):
		newHash, size, prefix = sha256.New, sha256.Size, "$5$"
	case strings.HasPrefix(setting, "$6$"):
		newHash, size, prefix = sha512.New, sha512.Size, "$6$"
	default:
		return "", fmt.Errorf("not a SHA-crypt setting")
	}

	rest := setting[len(prefix):]
	rounds, explicitRounds := shaDefaultRounds, false
	if after, ok := strings.CutPrefix(rest, "rounds="); ok {
		digits, tail, found := strings.Cut(after, "$")
		if !found {
			return "", fmt.Errorf("malformed rounds parameter")
		}
		n, err := strconv.Atoi(digits)
		if err != nil {
			return "", fmt.Errorf("malformed rounds parameter %q", digits)
		}
		// The spec CLAMPS rather than rejecting, and the clamped value is what
		// the stored hash was computed with — rejecting here would fail to
		// verify a password that a conforming crypt(3) accepts.
		rounds = min(max(n, shaMinRounds), shaMaxRounds)
		explicitRounds = true
		rest = tail
	}
	salt, _, _ := strings.Cut(rest, "$")
	if len(salt) > shaMaxSaltLen {
		salt = salt[:shaMaxSaltLen]
	}

	key := []byte(plain)
	saltB := []byte(salt)

	// B: the digest of key || salt || key.
	b := newHash()
	b.Write(key)
	b.Write(saltB)
	b.Write(key)
	sumB := b.Sum(nil)

	// A: key || salt, then |key| bytes taken cyclically from B, then one of
	// B/key per bit of |key| walking from the low bit upward.
	a := newHash()
	a.Write(key)
	a.Write(saltB)
	writeRepeated(a, sumB, len(key))
	for i := len(key); i > 0; i >>= 1 {
		if i&1 != 0 {
			a.Write(sumB)
		} else {
			a.Write(key)
		}
	}
	sumA := a.Sum(nil)

	// P: |key| bytes taken cyclically from the digest of key repeated |key|
	// times. S: |salt| bytes from the digest of salt repeated 16+A[0] times.
	dp := newHash()
	for range key {
		dp.Write(key)
	}
	p := cycle(dp.Sum(nil), len(key))

	ds := newHash()
	for range 16 + int(sumA[0]) {
		ds.Write(saltB)
	}
	s := cycle(ds.Sum(nil), len(saltB))

	// The stretching loop. Each round hashes P/S/C in an order determined by
	// the round index; the alternation is what makes the cost non-parallelizable
	// within a single evaluation.
	c := sumA
	for i := range rounds {
		h := newHash()
		if i&1 != 0 {
			h.Write(p)
		} else {
			h.Write(c)
		}
		if i%3 != 0 {
			h.Write(s)
		}
		if i%7 != 0 {
			h.Write(p)
		}
		if i&1 != 0 {
			h.Write(c)
		} else {
			h.Write(p)
		}
		c = h.Sum(nil)
	}

	out := &strings.Builder{}
	out.WriteString(prefix)
	if explicitRounds {
		fmt.Fprintf(out, "rounds=%d$", rounds)
	}
	out.WriteString(salt)
	out.WriteByte('$')
	out.WriteString(shaEncode(c, size))
	return out.String(), nil
}

// shaEncode renders the final digest in crypt's base64. The byte order is a
// fixed permutation of the digest, not a straight left-to-right read: for an
// n-byte digest the first n-1 bytes are emitted as triples (i, i+k, i+2k) mod
// (n-1) with i advancing by k+1 each group, and the last byte is emitted alone.
func shaEncode(digest []byte, size int) string {
	// mod is the number of digest bytes covered by the triples: SHA-512 keeps
	// ONE byte back for the final 2-character group, SHA-256 keeps TWO for its
	// final 3-character group, so the two moduli are not the same expression.
	var groups, step, offset, mod int
	switch size {
	case sha512.Size:
		groups, step, offset, mod = 21, 22, 21, 63
	case sha256.Size:
		groups, step, offset, mod = 10, 21, 10, 30
	default:
		return ""
	}

	var sb strings.Builder
	i := 0
	for range groups {
		b2 := digest[i]
		b1 := digest[(i+offset)%mod]
		b0 := digest[(i+2*offset)%mod]
		writeBase64(&sb, b2, b1, b0, 4)
		i = (i + step) % mod
	}
	if size == sha512.Size {
		writeBase64(&sb, 0, 0, digest[63], 2)
	} else {
		writeBase64(&sb, 0, digest[31], digest[30], 3)
	}
	return sb.String()
}

// --- MD5-crypt ($1$) ---------------------------------------------------------

const md5MaxSaltLen = 8

func md5Crypt(plain, setting string) (string, error) {
	if !strings.HasPrefix(setting, "$1$") {
		return "", fmt.Errorf("not an MD5-crypt setting")
	}
	salt, _, _ := strings.Cut(setting[3:], "$")
	if len(salt) > md5MaxSaltLen {
		salt = salt[:md5MaxSaltLen]
	}
	key := []byte(plain)
	saltB := []byte(salt)

	inner := md5.New()
	inner.Write(key)
	inner.Write(saltB)
	inner.Write(key)
	sumInner := inner.Sum(nil)

	h := md5.New()
	h.Write(key)
	h.Write([]byte("$1$"))
	h.Write(saltB)
	writeRepeated(h, sumInner, len(key))
	// One byte per bit of |key|, low bit first: a NUL for a set bit, the first
	// byte of the key for a clear one. (The historical implementation's quirk;
	// it is part of the format, not an accident that can be tidied away.)
	for i := len(key); i != 0; i >>= 1 {
		if i&1 != 0 {
			h.Write([]byte{0})
		} else {
			h.Write(key[:1])
		}
	}
	sum := h.Sum(nil)

	for i := range 1000 {
		r := md5.New()
		if i&1 != 0 {
			r.Write(key)
		} else {
			r.Write(sum)
		}
		if i%3 != 0 {
			r.Write(saltB)
		}
		if i%7 != 0 {
			r.Write(key)
		}
		if i&1 != 0 {
			r.Write(sum)
		} else {
			r.Write(key)
		}
		sum = r.Sum(nil)
	}

	var sb strings.Builder
	sb.WriteString("$1$")
	sb.WriteString(salt)
	sb.WriteByte('$')
	for _, g := range [][3]int{{0, 6, 12}, {1, 7, 13}, {2, 8, 14}, {3, 9, 15}, {4, 10, 5}} {
		writeBase64(&sb, sum[g[0]], sum[g[1]], sum[g[2]], 4)
	}
	writeBase64(&sb, 0, 0, sum[11], 2)
	return sb.String(), nil
}

// --- shared helpers ----------------------------------------------------------

// writeBase64 emits n characters of the 24-bit word b2<<16|b1<<8|b0, six bits
// at a time from the LEAST significant end — crypt's ordering, the reverse of
// standard base64.
func writeBase64(sb *strings.Builder, b2, b1, b0 byte, n int) {
	w := uint32(b2)<<16 | uint32(b1)<<8 | uint32(b0)
	for range n {
		sb.WriteByte(cryptAlphabet[w&0x3f])
		w >>= 6
	}
}

// writeRepeated writes n bytes taken cyclically from src.
func writeRepeated(h hash.Hash, src []byte, n int) {
	for n > len(src) {
		h.Write(src)
		n -= len(src)
	}
	h.Write(src[:n])
}

// cycle returns n bytes taken cyclically from src.
func cycle(src []byte, n int) []byte {
	out := make([]byte, 0, n)
	for len(out) < n {
		take := min(n-len(out), len(src))
		out = append(out, src[:take]...)
	}
	return out
}
