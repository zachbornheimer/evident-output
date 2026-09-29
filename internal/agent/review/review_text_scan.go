// Package review — text scanning helpers the text rules share: balanced delimiters, function bodies, line numbers, finding dedupe.
package review

import "strings"

// balancedArgs returns the substring between the parenthesis pair opening at
// openIdx (which must point at '(') and its matching close, plus the index
// just past the close paren.
func balancedArgs(src string, openIdx int) (args string, endIdx int, ok bool) {
	if openIdx < 0 || openIdx >= len(src) || src[openIdx] != '(' {
		return "", 0, false
	}
	depth := 0
	for i := openIdx; i < len(src); i++ {
		switch src[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return src[openIdx+1 : i], i + 1, true
			}
		}
	}
	return "", 0, false
}

// splitTopLevelArgs splits s on commas that are not nested inside
// parens/brackets/braces or a string literal.
func splitTopLevelArgs(s string) []string {
	var args []string
	depth := 0
	start := 0
	inStr := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '"' && (i == 0 || s[i-1] != '\\'):
			inStr = !inStr
		case inStr:
			continue
		case c == '(' || c == '[' || c == '{':
			depth++
		case c == ')' || c == ']' || c == '}':
			depth--
		case c == ',' && depth == 0:
			args = append(args, s[start:i])
			start = i + 1
		}
	}
	args = append(args, s[start:])
	return args
}

// funcBody is a function's brace-balanced source region and its byte offset.
type funcBody struct {
	body   string
	offset int
}

// firstFuncBody returns the first function body (brace-balanced) matching
// any of the given names, best-effort via textual scan.
func firstFuncBody(src string, names ...string) (string, int) {
	for _, name := range names {
		idx := strings.Index(src, "func "+name+"(")
		if idx < 0 {
			continue
		}
		if body, start, ok := balancedBraceBody(src, idx); ok {
			return body, start
		}
	}
	return "", -1
}

// allFuncBodies returns every top-level function body in src, best-effort.
func allFuncBodies(src string) []funcBody {
	var out []funcBody
	for i := 0; i < len(src); {
		idx := strings.Index(src[i:], "func ")
		if idx < 0 {
			break
		}
		idx += i
		if body, start, ok := balancedBraceBody(src, idx); ok {
			out = append(out, funcBody{body: body, offset: start})
			i = start + len(body)
		} else {
			i = idx + len("func ")
		}
	}
	return out
}

// balancedBraceBody finds the brace-balanced body of the function whose
// "func " keyword starts at fromIdx.
func balancedBraceBody(src string, fromIdx int) (body string, start int, ok bool) {
	braceIdx := strings.Index(src[fromIdx:], "{")
	if braceIdx < 0 {
		return "", 0, false
	}
	start = fromIdx + braceIdx
	depth := 0
	for i := start; i < len(src); i++ {
		switch src[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return src[start : i+1], start, true
			}
		}
	}
	return "", 0, false
}

// earliestIndex returns the smallest index of any marker in s, or -1 if none match.
func earliestIndex(s string, markers []string) int {
	idx, _ := earliestMarker(s, markers)
	return idx
}

// earliestMarker returns the smallest index of any marker in s and the
// matched marker text itself, or (-1, "") if none match.
func earliestMarker(s string, markers []string) (int, string) {
	best := -1
	bestMarker := ""
	for _, m := range markers {
		if idx := strings.Index(s, m); idx >= 0 && (best < 0 || idx < best) {
			best = idx
			bestMarker = m
		}
	}
	return best, bestMarker
}

// identBefore scans backward from idx over a dotted identifier chain
// (letters, digits, '_', '.') to recover the receiver name immediately
// preceding a call, e.g. "task" from "...\n  task.Doing(". Returns "" when
// no identifier character immediately precedes idx.
func identBefore(body string, idx int) string {
	end := idx
	start := end
	for start > 0 {
		c := body[start-1]
		if c == '_' || c == '.' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			start--
			continue
		}
		break
	}
	name := body[start:end]
	name = strings.TrimSuffix(name, ".")
	return name
}

// lineAt converts a byte offset into src to a 1-based line number.
func lineAt(src string, offset int) int {
	if offset < 0 || offset > len(src) {
		return 1
	}
	return 1 + strings.Count(src[:offset], "\n")
}

func hasRequired(fs []Finding) bool {
	for _, f := range fs {
		if f.Severity == "error" {
			return true
		}
	}
	// warnings also require recheck for agent loop
	return len(fs) > 0
}

func dedupe(fs []Finding) []Finding {
	seen := map[string]bool{}
	var out []Finding
	for _, f := range fs {
		k := f.RuleID + ":" + f.Message + ":" + f.File + ":" + itoa(f.Line)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, f)
	}
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [16]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
