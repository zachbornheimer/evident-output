package main

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
)

var hunkHeader = regexp.MustCompile(`^@@ -(\d+)(?:,\d+)? \+`)

// hunk is one patch hunk's pre-image: the lines it expects (context and
// removed) from the stated start, and which of those it removes.
type hunk struct {
	start   int
	pre     []string
	removed []int // indexes into pre
}

// patchHunks groups the hunks of a unified diff by the file they patch.
func patchHunks(patch []byte) map[string][]*hunk {
	hunks := map[string][]*hunk{}
	var file string
	var cur *hunk
	for line := range strings.SplitSeq(string(patch), "\n") {
		if m := hunkHeader.FindStringSubmatch(line); m != nil {
			start, _ := strconv.Atoi(m[1])
			cur = &hunk{start: start}
			hunks[file] = append(hunks[file], cur)
			continue
		}
		switch {
		case strings.HasPrefix(line, "diff --git"):
			cur = nil
		case cur == nil && strings.HasPrefix(line, patchSourcePrefix):
			file = diffPath(strings.TrimPrefix(line, patchSourcePrefix))
		case cur == nil:
		case strings.HasPrefix(line, "-"):
			cur.removed = append(cur.removed, len(cur.pre))
			cur.pre = append(cur.pre, line[1:])
		case strings.HasPrefix(line, " "):
			cur.pre = append(cur.pre, line[1:])
		}
	}
	return hunks
}

// patchRemovedLines maps each patched file to the line numbers, in the file as
// it stands, that the patch removes. A hunk's stated start can lag the file
// (git apply tolerates the offset), so each hunk is located by its text, at
// the match nearest its stated start. A file the patch only adds to has no entry.
func patchRemovedLines(patch []byte, readFile func(path string) ([]byte, error)) (map[string][]int, error) {
	removed := map[string][]int{}
	for path, hunks := range patchHunks(patch) {
		var content []string
		for _, h := range hunks {
			if len(h.removed) == 0 {
				continue
			}
			if content == nil {
				text, err := readFile(path)
				if err != nil {
					return nil, err
				}
				content = strings.Split(string(text), "\n")
			}
			at := h.locate(content)
			for _, i := range h.removed {
				removed[path] = append(removed[path], at+i)
			}
		}
		slices.Sort(removed[path])
	}
	return removed, nil
}

// locate returns the 1-based line where the hunk's pre-image matches content,
// nearest its stated start, or the stated start when it matches nowhere.
func (h *hunk) locate(content []string) int {
	for delta := 0; delta <= len(content); delta++ {
		for _, at := range []int{h.start - delta, h.start + delta} {
			if at >= 1 && at-1+len(h.pre) <= len(content) && slices.Equal(content[at-1:at-1+len(h.pre)], h.pre) {
				return at
			}
		}
	}
	return h.start
}
