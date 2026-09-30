package render

import (
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
// (verb, object). Foldable sections fold into the HIGHEST enclosing
// container whose whole subtree of ledger sections is foldable with that
// same pair, provided that container holds at least two such sections.
// So two manager Groups under one category Group fold to the category,
// while a category that also holds a different verb, a File or Exec row,
// or another object keeps its uniform sub-Groups as separate rows. A
// section that does not fold renders exactly as before. Sections with no
// rows never block a fold, so a failed item's empty section does not
// hide its siblings' row; the failure itself surfaces as its own Failed
// Task row.

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

// FoldEffectSections lays sources out as ledger sections of kind ("changed"
// or "planned"), in source order, with sibling same-effect sections folded
// per the rule above and every row aligned to one subject column. A folded
// row takes the place of its first member and keeps one record however
// many items it counts.
func FoldEffectSections(kind string, width int, sources []SectionSource) []EffectSection {
	folds := surveyContainers(sources)
	var (
		out      []EffectSection
		foldedAt = map[string]int{}
		subjects []string
	)
	for _, src := range sources {
		target, folded := foldTarget(src, folds)
		if !folded {
			subjects = append(subjects, src.Subject)
			if !src.Streamed {
				out = append(out, EffectSection{Kind: kind, Subject: src.Subject, Records: src.Records, IntendedVerb: src.IntendedVerb, Width: width})
			}
			continue
		}
		if at, seen := foldedAt[target.ID]; seen {
			out[at].Records[0].Quantity += quantityOf(src.Records)
			continue
		}
		k, _ := src.foldKind()
		foldedAt[target.ID] = len(out)
		subjects = append(subjects, target.Name)
		out = append(out, EffectSection{Kind: kind, Subject: target.Name, Width: width, GroupDigits: true,
			Records: []core.EffectRecord{{Verb: k.verb, Object: k.object, HasQty: true, Quantity: quantityOf(src.Records)}}})
	}
	alignSubjects(out, subjects)
	return out
}

// quantityOf is the total quantity a foldable section records.
func quantityOf(records []core.EffectRecord) int64 {
	var total int64
	for _, r := range records {
		total += r.Quantity
	}
	return total
}

// surveyContainers tallies what every container above a section holds: one
// pass, O(depth) per section.
func surveyContainers(sources []SectionSource) map[string]*containerFold {
	folds := map[string]*containerFold{}
	for _, src := range sources {
		if src.inert() {
			continue
		}
		k, foldable := src.foldKind()
		for _, c := range src.Containers {
			f, seen := folds[c.ID]
			if !seen {
				f = &containerFold{kind: k}
				folds[c.ID] = f
			}
			f.count++
			f.mixed = f.mixed || !foldable || f.kind != k
		}
	}
	return folds
}

// foldTarget is the container src folds into: the highest one above it
// whose subtree is uniform, when that subtree holds two or more sections.
func foldTarget(src SectionSource, folds map[string]*containerFold) (core.ContainerRef, bool) {
	if _, ok := src.foldKind(); !ok {
		return core.ContainerRef{}, false
	}
	var target core.ContainerRef
	for _, c := range src.Containers {
		if folds[c.ID].mixed {
			break
		}
		target = c
	}
	if target.ID == "" || folds[target.ID].count < 2 {
		return core.ContainerRef{}, false
	}
	return target, true
}

// alignSubjects sets the shared subject column to the widest subject the
// ledger shows; a lone row needs no alignment.
func alignSubjects(out []EffectSection, subjects []string) {
	if len(subjects) < 2 {
		return
	}
	widest := 0
	for _, s := range subjects {
		widest = max(widest, txt.Cells(s))
	}
	for i := range out {
		out[i].NameWidth = widest
	}
}
