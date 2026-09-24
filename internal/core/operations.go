package core

// OperationCounts tallies a Task's tracked operations (evo.File, evo.Exec)
// by how the operation manifest resolved them (§11, §39). The runtime
// counts them from the same events it streams as JSONL (§38).
type OperationCounts struct {
	// Current counts operations the manifest proved current and skipped.
	Current int
	// Executed counts operations Evo ran because no current proof existed.
	// In a dry run it counts the operations Evo would run.
	Executed int
	// BasisDrift counts the Executed operations whose Basis inputs changed
	// since the manifest recorded them.
	BasisDrift int
	// Changed counts finished operations that changed their tracked output.
	Changed int
	// Unchanged counts finished operations whose tracked output came out
	// identical, so nothing downstream needs to revalidate. A dry-run Exec
	// never runs, so it counts as neither Changed nor Unchanged.
	Unchanged int
}

// HitRate is the share of tracked operations the manifest proved current:
// the operation-manifest hit (no-op) rate.
func (o OperationCounts) HitRate() float64 { return ratio(o.Current, o.Consulted()) }

// BasisInvalidationRate is the share of tracked operations a Basis change
// invalidated.
func (o OperationCounts) BasisInvalidationRate() float64 {
	return ratio(o.BasisDrift, o.Consulted())
}

// ChangeRate is the share of finished operations that changed their
// tracked output.
func (o OperationCounts) ChangeRate() float64 { return ratio(o.Changed, o.finished()) }

// PropagationStoppedRate is the share of finished operations whose output
// came out identical, stopping propagation to dependents.
func (o OperationCounts) PropagationStoppedRate() float64 {
	return ratio(o.Unchanged, o.finished())
}

// Consulted is every tracked operation the manifest was asked about:
// those it proved Current plus those Evo Executed. It is the whole the
// hit and basis-invalidation rates divide by.
func (o OperationCounts) Consulted() int { return o.Current + o.Executed }

func (o OperationCounts) finished() int { return o.Changed + o.Unchanged }

func (o OperationCounts) plus(p OperationCounts) OperationCounts {
	return OperationCounts{
		Current:    o.Current + p.Current,
		Executed:   o.Executed + p.Executed,
		BasisDrift: o.BasisDrift + p.BasisDrift,
		Changed:    o.Changed + p.Changed,
		Unchanged:  o.Unchanged + p.Unchanged,
	}
}

// ratio is part/whole, or 0 when there is no whole to measure against.
func ratio(part, whole int) float64 {
	if whole == 0 {
		return 0
	}
	return float64(part) / float64(whole)
}
