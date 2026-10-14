package project

import "github.com/zachbornheimer/evident-output/v2/internal/record"

// Closed reports whether nothing more is expected to be declared into the
// collection id: the point a durable stream may write its block, since a
// member declared after it would make the block a part-truth.
func (r *Rows) Closed(id string) bool {
	g := r.byGroup[id]
	return g != nil && g.Node.MembershipClosed()
}

// RootLevel is the run's root level as the human projection reads it: each
// root Task by name alone (a flattened row is qualified against exactly
// those names), each root collection as a view sharing its records' slices,
// and the collection detailed as a full snapshot. Read it under the run's
// lock and never let it escape.
func (r *Rows) RootLevel(detailed string) record.Snapshot {
	var s record.Snapshot
	for _, t := range r.roots {
		s.Tasks = append(s.Tasks, record.TaskSnapshot{ID: string(t.ID), Name: t.Name})
	}
	for _, g := range r.groups {
		if g.ID == detailed {
			s.Collections = append(s.Collections, r.Collection(g.ID))
		} else {
			s.Collections = append(s.Collections, r.collectionView(g))
		}
	}
	return s
}

// collectionView is g with its members as views, recursively.
func (r *Rows) collectionView(g *DeclaredGroup) record.TasksSnapshot {
	ts := r.Header(g.ID)
	for _, t := range g.Tasks {
		ts.Tasks = append(ts.Tasks, r.View(t.ID))
	}
	for _, child := range g.Groups {
		ts.Collections = append(ts.Collections, r.collectionView(child))
	}
	return ts
}
