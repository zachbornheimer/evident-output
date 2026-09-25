package fix

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// diffContextLines is how many unchanged lines surround each hunk, the
// same default `diff -u` and `git diff` use — enough for `patch`/`git
// apply` to locate the hunk in a file that has drifted slightly since.
const diffContextLines = 3

// unifiedDiff renders a real unified diff between before and after: `@@
// -l,n +l,n @@` hunk headers, diffContextLines of surrounding context, and
// a module-relative path (never an absolute one, which a coordinate-based
// patch tool would refuse to apply against a different checkout). It
// line-diffs with a straightforward LCS (fine at source-file sizes; this
// is a CLI preview, not a hot path) instead of shelling out to a system
// `diff`, which the mise build may not have on PATH.
func unifiedDiff(filename, before, after string) string {
	if before == after {
		return ""
	}
	rel := diffRelPath(filename)
	// strings.Split on a trailing "\n" (every gofmt-formatted source file
	// has one) yields a phantom trailing "" element that is not a real
	// line. Left in, it throws off every hunk's line count against the
	// real file, which has no line after the last "\n". Both source
	// files here always end in a newline (renderEdits gofmt-formats the
	// result), so trimming it unconditionally is safe.
	a := strings.Split(strings.TrimSuffix(before, "\n"), "\n")
	b := strings.Split(strings.TrimSuffix(after, "\n"), "\n")
	ops := lcsDiff(a, b)
	hunks := hunksFromOps(ops, diffContextLines)

	var out strings.Builder
	fmt.Fprintf(&out, "--- a/%s\n+++ b/%s\n", rel, rel)
	for _, h := range hunks {
		fmt.Fprintf(&out, "@@ -%s +%s @@\n", hunkRange(h.aStart, h.aLines), hunkRange(h.bStart, h.bLines))
		for _, op := range h.ops {
			switch op.kind {
			case ' ':
				fmt.Fprintf(&out, " %s\n", op.line)
			case '-':
				fmt.Fprintf(&out, "-%s\n", op.line)
			case '+':
				fmt.Fprintf(&out, "+%s\n", op.line)
			}
		}
	}
	return out.String()
}

// diffRelPath returns filename relative to the nearest enclosing Go
// module root (the directory holding its go.mod), or filename unchanged
// when no module root is found — e.g. a file outside any module, or one
// the analyzer synthesized a path for. A module-relative path is what
// `git apply`/`patch -p1` expect; an absolute path baked into `--- a/...`
// only ever applies back onto the exact machine that produced it.
func diffRelPath(filename string) string {
	if !filepath.IsAbs(filename) {
		return filename
	}
	dir := filepath.Dir(filename)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			if rel, err := filepath.Rel(dir, filename); err == nil {
				return filepath.ToSlash(rel)
			}
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return filename
}

// hunkRange formats a hunk header's "start,count" field. A one-line hunk
// omits the count per the unified-diff convention (`@@ -8 +8 @@`, not
// `-8,1`), and a zero-line side (a hunk that only inserts, or only
// deletes, in that file) is rendered `start,0` with start meaning "after
// this line".
func hunkRange(start, count int) string {
	if count == 1 {
		return fmt.Sprintf("%d", start)
	}
	return fmt.Sprintf("%d,%d", start, count)
}

type diffOp struct {
	kind rune // '-' removed, '+' added, ' ' unchanged
	line string
}

type hunk struct {
	aStart, aLines int
	bStart, bLines int
	ops            []diffOp
}

// hunksFromOps groups a full context+change op stream (as lcsDiff
// returns it) into unified-diff hunks: consecutive changes within
// 2*context unchanged lines of each other share one hunk, and each
// hunk keeps at most `context` lines of unchanged text on either side of
// its changes.
func hunksFromOps(ops []diffOp, context int) []hunk {
	var changeIdx []int
	for i, op := range ops {
		if op.kind != ' ' {
			changeIdx = append(changeIdx, i)
		}
	}
	if len(changeIdx) == 0 {
		return nil
	}

	// Cluster change indices: a new cluster starts whenever the gap since
	// the previous change exceeds 2*context unchanged lines.
	type span struct{ start, end int } // inclusive op indices spanning one cluster's changes
	clusters := []span{{changeIdx[0], changeIdx[0]}}
	for _, idx := range changeIdx[1:] {
		last := &clusters[len(clusters)-1]
		if idx-last.end <= 2*context {
			last.end = idx
		} else {
			clusters = append(clusters, span{idx, idx})
		}
	}

	// aLineAt[i]/bLineAt[i] is the 1-based source-line number ops[i]
	// corresponds to in a/b, so a hunk header can be built directly from
	// whichever op index it starts at.
	aLineAt := make([]int, len(ops))
	bLineAt := make([]int, len(ops))
	a, b := 1, 1
	for i, op := range ops {
		aLineAt[i], bLineAt[i] = a, b
		switch op.kind {
		case ' ':
			a++
			b++
		case '-':
			a++
		case '+':
			b++
		}
	}

	hunks := make([]hunk, 0, len(clusters))
	for _, c := range clusters {
		start, end := c.start-context, c.end+context
		if start < 0 {
			start = 0
		}
		if end >= len(ops) {
			end = len(ops) - 1
		}
		h := hunk{aStart: aLineAt[start], bStart: bLineAt[start]}
		for i := start; i <= end; i++ {
			op := ops[i]
			h.ops = append(h.ops, op)
			switch op.kind {
			case ' ':
				h.aLines++
				h.bLines++
			case '-':
				h.aLines++
			case '+':
				h.bLines++
			}
		}
		hunks = append(hunks, h)
	}
	return hunks
}

// lcsDiff computes a line-level diff via a straightforward dynamic-program
// LCS, returning every line — changed and unchanged — tagged with its op
// kind, so hunksFromOps has the context it needs to build real hunks.
func lcsDiff(a, b []string) []diffOp {
	n, m := len(a), len(b)
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}
	var ops []diffOp
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			ops = append(ops, diffOp{' ', a[i]})
			i++
			j++
		case dp[i+1][j] >= dp[i][j+1]:
			ops = append(ops, diffOp{'-', a[i]})
			i++
		default:
			ops = append(ops, diffOp{'+', b[j]})
			j++
		}
	}
	for ; i < n; i++ {
		ops = append(ops, diffOp{'-', a[i]})
	}
	for ; j < m; j++ {
		ops = append(ops, diffOp{'+', b[j]})
	}
	return ops
}
