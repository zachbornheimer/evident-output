package fix

import (
	"fmt"
	"strings"
)

// unifiedDiff renders a minimal unified diff between before and after —
// enough for a human or `patch` to apply, not a byte-identical match for
// `diff -u`. It line-diffs with a straightforward LCS (fine at source-file
// sizes; this is a CLI preview, not a hot path) instead of shelling out to
// a system `diff`, which the mise build may not have on PATH.
func unifiedDiff(filename, before, after string) string {
	if before == after {
		return ""
	}
	a := strings.Split(before, "\n")
	b := strings.Split(after, "\n")
	ops := lcsDiff(a, b)

	var out strings.Builder
	fmt.Fprintf(&out, "--- a/%s\n+++ b/%s\n", filename, filename)
	for _, op := range ops {
		switch op.kind {
		case '-':
			fmt.Fprintf(&out, "-%s\n", op.line)
		case '+':
			fmt.Fprintf(&out, "+%s\n", op.line)
		}
	}
	return out.String()
}

type diffOp struct {
	kind rune // '-' removed, '+' added, ' ' unchanged (dropped from output)
	line string
}

// lcsDiff computes a line-level diff via a straightforward dynamic-program
// LCS, returning only the changed lines (unchanged runs are collapsed by
// the caller not printing ' ' ops).
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
