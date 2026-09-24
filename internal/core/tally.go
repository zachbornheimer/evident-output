package core

// Tally is the counted partition of disposition records (TaskHandle.Kept
// or TaskHandle.Skipped): how many, split by Reason in first-seen order,
// with the item names under each reason. It is the one owner of that
// count, whether the records sit on a single Task or come from a Group's
// per-item children, so the headline and its reason parts always sum.
//
// Building one is a single O(records) pass with no sorting, so a renderer
// may rebuild it every frame.
type Tally struct {
	reasons []ReasonTally
	index   map[string]int
	causes  []string
	total   int
}

// ReasonTally is one Reason's share of a Tally, with its item names in
// record order.
type ReasonTally struct {
	Reason string
	Names  []string
}

// Add counts rec under its Reason.
func (t *Tally) Add(rec TaxonomyRecord) {
	if t.index == nil {
		t.index = make(map[string]int)
	}
	i, seen := t.index[rec.Reason]
	if !seen {
		i = len(t.reasons)
		t.index[rec.Reason] = i
		t.reasons = append(t.reasons, ReasonTally{Reason: rec.Reason})
	}
	t.reasons[i].Names = append(t.reasons[i].Names, rec.Name)
	t.causes = append(t.causes, rec.Causes...)
	t.total++
}

// AddAll counts every record in records.
func (t *Tally) AddAll(records []TaxonomyRecord) {
	for _, rec := range records {
		t.Add(rec)
	}
}

// Total is how many records the Tally counted.
func (t Tally) Total() int { return t.total }

// Reasons is the partition, in the order each Reason was first recorded.
func (t Tally) Reasons() []ReasonTally { return t.reasons }

// Causes is every counted record's evidence text, in record order.
func (t Tally) Causes() []string { return t.causes }

// TallyOf counts records.
func TallyOf(records []TaxonomyRecord) Tally {
	var t Tally
	t.AddAll(records)
	return t
}

// Dispositions are a Task's or a Group's two tallies.
type Dispositions struct {
	Skipped Tally
	Kept    Tally
}

// Empty reports whether nothing was skipped or kept.
func (d Dispositions) Empty() bool { return d.Skipped.Total() == 0 && d.Kept.Total() == 0 }

// AddTask counts t's own Skipped and Kept records.
func (d *Dispositions) AddTask(t TaskSnapshot) {
	d.Skipped.AddAll(t.Skipped)
	d.Kept.AddAll(t.Kept)
}
