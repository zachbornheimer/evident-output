package render

import (
	"fmt"

	"github.com/zachbornheimer/evident-output/internal/core"
	txt "github.com/zachbornheimer/evident-output/internal/text"
)

// The human ledger states a category once. A Group whose per-item Tasks
// each record the same Effect (verb and object) would otherwise print one
// row per item; it prints one row for the container, counted per item:
// "[changed] centralize packages  centralized 1,663 packages". Like the
// Kept/Skipped tally (disposition_items.go), this is a renderer concern:
// JSON and JSONL keep every per-item Effect, and the model keeps every
// section.
//
// Fold rule. A section is foldable when it sits under a container, is
// not already streamed, and every record is a counted Effect sharing one
// (verb, object). Two levels, tried in order:
//
//  1. Uniform lift. A foldable section folds into the HIGHEST enclosing
//     container whose whole subtree of ledger sections is foldable with
//     that same pair, provided that container holds at least two. Two
//     manager Groups under one category Group fold to the category.
//  2. Direct siblings. In a container that is not uniform, the foldable
//     sections that are its own direct children fold per (verb, object):
//     each pair with two or more members is one row named for the
//     container. A single different sibling therefore never stops the rest
//     folding.
//
// A section that does not fold renders exactly as before. Sections with no
// rows never block a fold, so a failed item's empty section does not hide
// its siblings' row; the failure itself surfaces as its own Failed Task row.

// SectionSource is one ledger section as the fold reads it.
type SectionSource struct {
	Subject      string
	Records      []core.EffectRecord
	IntendedVerb string
	// Containers is the section owner's enclosing containers, nearest first.
	Containers core.ContainerPath
	// Streamed marks a section already written at its Task's resolution: it
	// still aligns its siblings but is not written again, and never folds.
	Streamed bool
}

// effectKind is the (verb, object) an Effect section records.
type effectKind struct{ verb, object string }

// foldKind is the single effectKind s records, and whether s qualifies to
// fold at all.
func (s SectionSource) foldKind() (effectKind, bool) {
	if s.Streamed || len(s.Containers) == 0 || len(s.Records) == 0 {
		return effectKind{}, false
	}
	kind := effectKind{s.Records[0].Verb, s.Records[0].Object}
	for _, r := range s.Records {
		if !r.HasQty || (effectKind{r.Verb, r.Object}) != kind {
			return effectKind{}, false
		}
	}
	return kind, true
}

// inert reports whether s says nothing, so it neither folds nor blocks one.
func (s SectionSource) inert() bool { return !s.Streamed && len(s.Records) == 0 }

// containerFold is what one container's subtree of sections holds.
type containerFold struct {
	kind  effectKind
	count int
	// mixed is true once the subtree holds anything but foldable sections
	// of one kind.
	mixed bool
}

// foldKey names one folded row: a container and the Effect it counts.
type foldKey struct {
	container string
	kind      effectKind
}

// foldSurvey is what the fold learns about the ledger before laying it out.
type foldSurvey struct {
	// subtrees is what every container above a section holds.
	subtrees map[string]*containerFold
	// direct counts the foldable sections that are a container's own
	// children, per Effect.
	direct map[foldKey]int
}

// FoldEffectSections lays sources out as ledger sections of kind ("changed"
// or "planned"), in source order, with sibling same-effect sections folded
// per the rule above and every row aligned to one subject column. A folded
// row takes the place of its first member and keeps one record however
// many items it counts.
func FoldEffectSections(kind string, width int, sources []SectionSource) []EffectSection {
	survey := surveyFold(sources)
	var (
		out      []EffectSection
		foldedAt = map[foldKey]int{}
		shown    []shownSubject
	)
	for _, src := range sources {
		target, key, folded := survey.target(src)
		if !folded {
			shown = append(shown, shownSubject{text: src.Subject, out: -1})
			if !src.Streamed {
				shown[len(shown)-1].out = len(out)
				out = append(out, EffectSection{Kind: kind, Subject: src.Subject, Records: src.Records, IntendedVerb: src.IntendedVerb, Width: width})
			}
			continue
		}
		if at, seen := foldedAt[key]; seen {
			out[at].Records[0].Quantity += quantityOf(src.Records)
			continue
		}
		foldedAt[key] = len(out)
		shown = append(shown, shownSubject{text: target[0].Name, out: len(out), container: target})
		out = append(out, EffectSection{Kind: kind, Subject: target[0].Name, Width: width,
			Records: []core.EffectRecord{{Verb: key.kind.verb, Object: key.kind.object, HasQty: true, Quantity: quantityOf(src.Records)}}})
	}
	qualifyDuplicates(out, shown)
	alignSubjects(out, shown)
	return out
}

// shownSubject is one row's subject as the ledger shows it. out is its
// index in the written sections, -1 for a streamed row that only aligns.
// container is set for a folded row: the path from the container it names.
type shownSubject struct {
	text      string
	out       int
	container core.ContainerPath
}

// qualifyDuplicates names a folded row by its container path when another
// row, folded or not, shows the same subject: two "branches" categories
// under different repos read "alpha › branches" and "beta › branches". Rows
// of one container (one per Effect) already share a subject knowingly.
func qualifyDuplicates(out []EffectSection, shown []shownSubject) {
	owner := map[string]string{}
	duplicated := map[string]bool{}
	for i, s := range shown {
		id := fmt.Sprintf("row %d", i)
		if len(s.container) > 0 {
			id = s.container[0].ID
		}
		if first, seen := owner[s.text]; seen && first != id {
			duplicated[s.text] = true
		}
		owner[s.text] = id
	}
	for i, s := range shown {
		if len(s.container) == 0 || !duplicated[s.text] {
			continue
		}
		shown[i].text = s.container[1:].Qualify(s.text)
		out[s.out].Subject = shown[i].text
	}
}

// quantityOf is the total quantity a foldable section records.
func quantityOf(records []core.EffectRecord) int64 {
	var total int64
	for _, r := range records {
		total += r.Quantity
	}
	return total
}

// surveyFold tallies what every container above a section holds: one pass,
// O(depth) per section.
func surveyFold(sources []SectionSource) foldSurvey {
	survey := foldSurvey{subtrees: map[string]*containerFold{}, direct: map[foldKey]int{}}
	for _, src := range sources {
		if src.inert() {
			continue
		}
		k, foldable := src.foldKind()
		if foldable {
			survey.direct[foldKey{src.Containers[0].ID, k}]++
		}
		for _, c := range src.Containers {
			f, seen := survey.subtrees[c.ID]
			if !seen {
				f = &containerFold{kind: k}
				survey.subtrees[c.ID] = f
			}
			f.count++
			f.mixed = f.mixed || !foldable || f.kind != k
		}
	}
	return survey
}

// target is the container src folds into and the row it joins, when it
// folds at all.
func (v foldSurvey) target(src SectionSource) (core.ContainerPath, foldKey, bool) {
	kind, ok := src.foldKind()
	if !ok {
		return nil, foldKey{}, false
	}
	if top, lifted := v.highestUniform(src); lifted {
		return top, foldKey{top[0].ID, kind}, true
	}
	if nearest := src.Containers[0]; v.direct[foldKey{nearest.ID, kind}] >= 2 {
		return src.Containers, foldKey{nearest.ID, kind}, true
	}
	return nil, foldKey{}, false
}

// highestUniform is the path from the highest container above src whose
// whole subtree is foldable with src's Effect, when it holds two or more
// sections.
func (v foldSurvey) highestUniform(src SectionSource) (core.ContainerPath, bool) {
	top := -1
	for i, c := range src.Containers {
		if v.subtrees[c.ID].mixed {
			break
		}
		top = i
	}
	if top < 0 || v.subtrees[src.Containers[top].ID].count < 2 {
		return nil, false
	}
	return src.Containers[top:], true
}

// alignSubjects sets the shared subject column to the widest subject the
// ledger shows; a lone row needs no alignment.
func alignSubjects(out []EffectSection, shown []shownSubject) {
	if len(shown) < 2 {
		return
	}
	widest := 0
	for _, s := range shown {
		widest = max(widest, txt.Cells(s.text))
	}
	for i := range out {
		out[i].NameWidth = widest
	}
}
