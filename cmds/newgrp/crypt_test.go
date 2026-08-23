package newgrpcmd

import (
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

// Every $1$/$5$/$6$ vector below was produced by an INDEPENDENT implementation
// (`openssl passwd -1 / -5 / -6`, OpenSSL 3.6.3) rather than by this code, so
// the table is a cross-check and not a snapshot of whatever the code happened
// to compute. The $6$saltstring and $5$saltstring pairs are also the published
// SHA-crypt specification vectors.
var cryptVectors = []struct {
	name  string
	plain string
	hash  string
}{
	{
		"sha512 spec vector",
		"Hello world!",
		"$6$saltstring$svn8UoSVapNtMuq1ukKS4tPQd8iKwSMHWjl/O817G3uBnIFNjnQJuesI68u4OTLiBFdcbYEdFCoEOfaS35inz1",
	},
	{
		"sha512 short password",
		"secret",
		"$6$saltstring$AIsRs/Ee56G/tC8MEHhvReZTfx8u3rXXMl6eYrjCG9ibix19DxoMBLogdTET5Ukw9Sf7eZTITsuk0Ry5qulYz.",
	},
	{
		// Longer than the digest, which exercises the cyclic P/B fill.
		"sha512 password longer than the digest",
		"a-very-long-password-exceeding-sixty-four-bytes-in-total-length!!",
		"$6$saltstring$zOCDDWULyhazVvj7W5F.6VMD1ZfCRDc1BCh3irJ1rJ2W.zvPOs/Laj55BhzlaHWLVEzrcQlwl5z0BuBrGhQx61",
	},
	{
		// A two-character salt: the S block is shorter than one digest, so the
		// truncation path is exercised too.
		"sha512 short salt",
		"pw",
		"$6$ab$td.Yv18i64fuHSCSFfhcg9cvGN/q.1N72dPYEBvvVyQIQbX2mVX2dRrHRg.vLlqSchKS1d4urAkRbfv.vYLNE1",
	},
	{
		"sha256 spec vector",
		"Hello world!",
		"$5$saltstring$5B8vYYiY.CVt1RlTTf8KbXBH3hsxY/GNooZaBBGWEc5",
	},
	{
		"sha256 short password",
		"secret",
		"$5$saltstring$C3o4O1TC6aRHF4FI.QSZMXtHbaj2gSXr4sUc/3NcUi.",
	},
	{
		"sha256 password longer than the digest",
		"a-very-long-password-exceeding-sixty-four-bytes-in-total-length!!",
		"$5$saltstring$d4SbbpIOov93hyEuBZ.5e2UpSW2AU5wL8/3Ln8IMoZD",
	},
	{
		// MD5-crypt truncates the salt at eight characters, so "saltstring"
		// hashes as "saltstri" and the stored setting says so.
		"md5 salt truncated to eight characters",
		"Hello world!",
		"$1$saltstri$YMyguxXMBpd2TEZ.vS/3q1",
	},
	{
		"md5 short password",
		"secret",
		"$1$saltstri$WHZcnT3IOdSrMvizOq7Ht1",
	},
	{
		"md5 empty password",
		"",
		"$1$saltstri$ciR2otLVXV8I9sOPWbLTc1",
	},
	{
		"md5 password longer than the digest",
		"a-very-long-password-exceeding-sixty-four-bytes-in-total-length!!",
		"$1$saltstri$2JSElXJ1wAErUtwgXNf/v0",
	},
	{
		"md5 short salt",
		"pw",
		"$1$ab$b2XAKzcGJvTR.javvk3280",
	},
}

func TestVerifyCryptAcceptsTheCorrectPassword(t *testing.T) {
	for _, tc := range cryptVectors {
		t.Run(tc.name, func(t *testing.T) {
			ok, err := verifyCrypt(tc.hash, tc.plain)
			if err != nil {
				t.Fatalf("verifyCrypt: %v", err)
			}
			if !ok {
				t.Errorf("the correct password was rejected for %s", tc.hash)
			}
		})
	}
}

func TestVerifyCryptRejectsTheWrongPassword(t *testing.T) {
	for _, tc := range cryptVectors {
		t.Run(tc.name, func(t *testing.T) {
			ok, err := verifyCrypt(tc.hash, tc.plain+"x")
			if err != nil {
				t.Fatalf("verifyCrypt: %v", err)
			}
			if ok {
				t.Errorf("a wrong password was accepted for %s", tc.hash)
			}
		})
	}
}

// The rounds= parameter changes the result, so a hash that carries one must be
// verified with it — reading it and ignoring it would silently reject every
// correct password on a hardened system.
func TestRoundsParameterIsHonoured(t *testing.T) {
	// Recomputing the default-rounds vector with an explicit rounds=5000 must
	// give the same digest, and a different round count must give a different
	// one. That pins the parameter as load-bearing without needing an external
	// hash at a nonstandard cost.
	base := "$6$saltstring$svn8UoSVapNtMuq1ukKS4tPQd8iKwSMHWjl/O817G3uBnIFNjnQJuesI68u4OTLiBFdcbYEdFCoEOfaS35inz1"
	explicit, err := shaCrypt("Hello world!", "$6$rounds=5000$saltstring$")
	if err != nil {
		t.Fatal(err)
	}
	if _, digest, _ := strings.Cut(strings.TrimPrefix(explicit, "$6$rounds=5000$saltstring$"), "$"); digest != "" {
		t.Fatalf("unexpected extra field in %q", explicit)
	}
	if !strings.HasSuffix(explicit, strings.TrimPrefix(base, "$6$saltstring$")) {
		t.Errorf("rounds=5000 must reproduce the default-rounds digest:\n got %q\nwant suffix %q",
			explicit, strings.TrimPrefix(base, "$6$saltstring$"))
	}

	other, err := shaCrypt("Hello world!", "$6$rounds=1234$saltstring$")
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasSuffix(other, strings.TrimPrefix(base, "$6$saltstring$")) {
		t.Error("a different round count must produce a different digest")
	}
	// Below the minimum the count CLAMPS, so 1234 (already clamped to 1234? no,
	// the floor is 1000) and 10 must agree with the floor rather than fail.
	clamped, err := shaCrypt("Hello world!", "$6$rounds=10$saltstring$")
	if err != nil {
		t.Fatal(err)
	}
	floor, err := shaCrypt("Hello world!", "$6$rounds=1000$saltstring$")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimPrefix(clamped, "$6$rounds=1000$") != strings.TrimPrefix(floor, "$6$rounds=1000$") {
		t.Error("a round count below the minimum must clamp to the minimum, not fail")
	}
}

func TestVerifyCryptBcrypt(t *testing.T) {
	h, err := bcrypt.GenerateFromPassword([]byte("hunter2"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	ok, err := verifyCrypt(string(h), "hunter2")
	if err != nil || !ok {
		t.Errorf("bcrypt accept: ok=%v err=%v", ok, err)
	}
	ok, err = verifyCrypt(string(h), "wrong")
	if err != nil || ok {
		t.Errorf("bcrypt reject: ok=%v err=%v", ok, err)
	}
}

// A scheme this code cannot evaluate must ERROR, never return false. False
// means "authenticated and wrong" and would send an operator hunting a typo
// that does not exist; the caller turns the error into "cannot verify".
func TestUnsupportedSchemeIsAnErrorNotAMismatch(t *testing.T) {
	for _, h := range []string{
		"$y$j9T$salt$digest",        // yescrypt
		"$7$C6..../....salt$digest", // scrypt
		"abJnggxhB/yWI",             // historical DES crypt
		"",                          // empty field reaching the verifier
	} {
		ok, err := verifyCrypt(h, "anything")
		if err == nil {
			t.Errorf("verifyCrypt(%q) must report that it cannot evaluate the hash", h)
		}
		if ok {
			t.Errorf("verifyCrypt(%q) must never return true for an unevaluable hash", h)
		}
	}
}
