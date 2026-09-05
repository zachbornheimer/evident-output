package adopt

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// InventoryPage walks dir and returns one cursor-window of the plan.
func InventoryPage(dir string, opts InventoryOptions) (Page, error) {
	plan, err := Inventory(dir)
	if err != nil {
		return Page{}, err
	}
	return plan.Page(opts)
}

// Page slices p into one findings window. An empty cursor starts at the
// facade page when facades exist, otherwise the first ladder rung.
func (p Plan) Page(opts InventoryOptions) (Page, error) {
	cur, err := decodeCursor(opts.Cursor)
	if err != nil {
		return Page{}, err
	}
	limit := pageLimit(opts.Limit)
	out := Page{Directory: p.Directory, Caveat: p.Caveat, Facades: p.Facades}

	if len(p.Facades) > 0 && !cur.AfterFacades && cur.Rung == "" {
		out.Findings = nil
		out.NextAction = NextActionFacade
		out.Remaining = len(p.Findings)
		if len(p.Findings) > 0 {
			out.NextCursor = encodeCursor(pageCursor{AfterFacades: true})
		}
		return out, nil
	}

	grouped := findingsByRung(p.Findings)
	rungs := rungsFromCursor(grouped, cur)
	if len(rungs) == 0 {
		out.NextAction = NextActionClean
		out.Facades = p.Facades
		return out, nil
	}

	rung := rungs[0]
	bucket := grouped[rung]
	offset := 0
	if cur.Rung == string(rung) {
		offset = cur.Offset
		if offset > len(bucket) {
			offset = len(bucket)
		}
	}
	end := min(offset+limit, len(bucket))
	out.Findings = bucket[offset:end]
	out.Rung = rung
	out.Remaining = (len(bucket) - end) + countRungs(grouped, rungs[1:])
	out.NextAction = rungAction(rung, out.Remaining)
	if len(bucket) > end {
		out.NextCursor = encodeCursor(pageCursor{AfterFacades: true, Rung: string(rung), Offset: end})
	} else if len(rungs) > 1 {
		out.NextCursor = encodeCursor(pageCursor{AfterFacades: true, Rung: string(rungs[1]), Offset: 0})
	}
	return out, nil
}

type pageCursor struct {
	AfterFacades bool   `json:"af,omitempty"`
	Rung         string `json:"r,omitempty"`
	Offset       int    `json:"o,omitempty"`
}

func decodeCursor(s string) (pageCursor, error) {
	if s == "" {
		return pageCursor{}, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return pageCursor{}, fmt.Errorf("invalid cursor")
	}
	var cur pageCursor
	if err := json.Unmarshal(raw, &cur); err != nil {
		return pageCursor{}, fmt.Errorf("invalid cursor")
	}
	return cur, nil
}

func encodeCursor(cur pageCursor) string {
	raw, err := json.Marshal(cur)
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

func pageLimit(n int) int {
	if n <= 0 || n > DefaultPageSize {
		return DefaultPageSize
	}
	return n
}

func findingsByRung(findings []Finding) map[Rung][]Finding {
	grouped := map[Rung][]Finding{}
	for _, f := range findings {
		grouped[f.Rung] = append(grouped[f.Rung], f)
	}
	return grouped
}

func rungsFromCursor(grouped map[Rung][]Finding, cur pageCursor) []Rung {
	var out []Rung
	started := cur.Rung == ""
	for _, r := range ladderOrder {
		if !started {
			if string(r) == cur.Rung {
				started = true
			} else {
				continue
			}
		}
		if len(grouped[r]) == 0 {
			continue
		}
		if string(r) == cur.Rung && cur.Offset >= len(grouped[r]) {
			continue
		}
		out = append(out, r)
	}
	return out
}

func countRungs(grouped map[Rung][]Finding, rungs []Rung) int {
	n := 0
	for _, r := range rungs {
		n += len(grouped[r])
	}
	return n
}

func rungAction(rung Rung, remaining int) string {
	if remaining > 0 {
		return "migrate " + string(rung) + ", then re-call with next_cursor"
	}
	return "migrate " + string(rung)
}
