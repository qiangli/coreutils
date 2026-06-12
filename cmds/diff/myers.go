// Myers O(ND) diff engine in linear space.
//
// Implements the divide-and-conquer formulation of Eugene W. Myers,
// "An O(ND) Difference Algorithm and Its Variations" (Algorithmica,
// 1986), section 4b: find the middle of an optimal edit path by
// running the forward and reverse D-loops until they overlap, then
// recurse on the two halves. Written fresh for this repository;
// priorart/aict/tools/diff was reviewed but embeds only a quadratic
// LCS table, so nothing was adapted from it.
package diffcmd

type opKind byte

const (
	opEq opKind = iota
	opDel
	opIns
)

// myersOps returns a minimal edit script transforming a into b: a
// sequence of opEq/opDel/opIns that consumes every element of a (Eq,
// Del) and of b (Eq, Ins) in order.
func myersOps(a, b []int) []opKind {
	out := make([]opKind, 0, len(a)+len(b))
	myersRec(a, b, &out)
	return out
}

func myersRec(a, b []int, out *[]opKind) {
	// Strip common prefix and suffix; they are pure snakes.
	p := 0
	for p < len(a) && p < len(b) && a[p] == b[p] {
		p++
	}
	s := 0
	for s < len(a)-p && s < len(b)-p && a[len(a)-1-s] == b[len(b)-1-s] {
		s++
	}
	for i := 0; i < p; i++ {
		*out = append(*out, opEq)
	}
	ma, mb := a[p:len(a)-s], b[p:len(b)-s]
	switch {
	case len(ma) == 0:
		for range mb {
			*out = append(*out, opIns)
		}
	case len(mb) == 0:
		for range ma {
			*out = append(*out, opDel)
		}
	default:
		x, y, ok := bisect(ma, mb)
		if !ok || (x == 0 && y == 0) || (x == len(ma) && y == len(mb)) {
			// Degenerate split. Cannot happen on the minimal-path
			// invariant (after prefix/suffix stripping the edit
			// distance is >= 2), kept as a safety net: emit a full
			// replacement rather than recurse forever.
			for range ma {
				*out = append(*out, opDel)
			}
			for range mb {
				*out = append(*out, opIns)
			}
		} else {
			myersRec(ma[:x], mb[:y], out)
			myersRec(ma[x:], mb[y:], out)
		}
	}
	for i := 0; i < s; i++ {
		*out = append(*out, opEq)
	}
}

// bisect finds a point (x, y) on an optimal edit path between a and b
// by searching forward from (0,0) and backward from (n,m)
// simultaneously and stopping at first overlap. O(N+M) space.
func bisect(a, b []int) (int, int, bool) {
	n, m := len(a), len(b)
	maxD := (n + m + 1) / 2
	vOff := maxD
	vLen := 2*maxD + 4
	v1 := make([]int, vLen) // forward: furthest x by diagonal k
	v2 := make([]int, vLen) // reverse: furthest x (from the end) by diagonal k
	for i := range v1 {
		v1[i] = -1
		v2[i] = -1
	}
	v1[vOff+1] = 0
	v2[vOff+1] = 0
	delta := n - m
	front := delta%2 != 0 // overlap detected on the forward pass when D is odd
	k1start, k1end := 0, 0
	k2start, k2end := 0, 0
	for d := 0; d <= maxD; d++ {
		// Forward path.
		for k1 := -d + k1start; k1 <= d-k1end; k1 += 2 {
			ko := vOff + k1
			var x int
			if k1 == -d || (k1 != d && v1[ko-1] < v1[ko+1]) {
				x = v1[ko+1]
			} else {
				x = v1[ko-1] + 1
			}
			y := x - k1
			for x < n && y < m && a[x] == b[y] {
				x++
				y++
			}
			v1[ko] = x
			switch {
			case x > n:
				k1end += 2
			case y > m:
				k1start += 2
			case front:
				ko2 := vOff + delta - k1
				if ko2 >= 0 && ko2 < vLen && v2[ko2] != -1 {
					if x >= n-v2[ko2] {
						return x, y, true
					}
				}
			}
		}
		// Reverse path (coordinates measured from the far corner).
		for k2 := -d + k2start; k2 <= d-k2end; k2 += 2 {
			ko := vOff + k2
			var x int
			if k2 == -d || (k2 != d && v2[ko-1] < v2[ko+1]) {
				x = v2[ko+1]
			} else {
				x = v2[ko-1] + 1
			}
			y := x - k2
			for x < n && y < m && a[n-x-1] == b[m-y-1] {
				x++
				y++
			}
			v2[ko] = x
			switch {
			case x > n:
				k2end += 2
			case y > m:
				k2start += 2
			case !front:
				ko1 := vOff + delta - k2
				if ko1 >= 0 && ko1 < vLen && v1[ko1] != -1 {
					x1 := v1[ko1]
					y1 := x1 - (delta - k2)
					if x1 >= n-x {
						return x1, y1, true
					}
				}
			}
		}
	}
	return 0, 0, false
}
