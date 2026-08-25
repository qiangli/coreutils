package touchcmd

import (
	"path/filepath"
	"testing"
	"time"
)

// Issue 7 -t: SS is [00,60], and an SS of 60 that does not name a real
// leap second means one second after SS=59 — including at 23:59, where
// the result rolls into the next day (or month, or year).
func TestTouchStampSecond60RollsForward(t *testing.T) {
	cases := []struct {
		stamp string
		want  time.Time
	}{
		{"202601021317.60", time.Date(2026, 1, 2, 13, 18, 0, 0, time.UTC)},
		{"202601022359.60", time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)},
		{"202612312359.60", time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)},
	}
	for _, c := range cases {
		dir := t.TempDir()
		_, errb, code := runToolEnv(t, dir, []string{"TZ=UTC0"}, "-t", c.stamp, "f")
		if code != 0 {
			t.Errorf("-t %s: code=%d err=%q", c.stamp, code, errb)
			continue
		}
		if got := mtime(t, filepath.Join(dir, "f")); got.Unix() != c.want.Unix() {
			t.Errorf("-t %s: mtime=%v want %v", c.stamp, got, c.want)
		}
	}
}
