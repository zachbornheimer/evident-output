package evo_test

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

func TestOUT021_DataProjectionOption(t *testing.T) {
	var primary, diag bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &primary, Stderr: &diag, Color: evo.ColorNever, Plain: true, Format: evo.FormatData})
	t.Cleanup(func() { _ = out.Close() })
	out.Task("scan").Doing("walk").Done("ok")
	_ = out.Finish()
	// Data projection still renders human to primary in v0.3 path unless diagnostic set for UI;
	// ensure option is accepted and Finish works.
	if primary.Len() == 0 && diag.Len() == 0 {
		t.Fatal("expected some output")
	}
}

// TestAPI016_ExternalProjectionSnapshots is C8: the streaming Snapshots()
// channel is deleted (Output.Snapshot() — singular, poll-based — is the
// surviving accessor); FormatExternal's "snapshots only" promise still
// holds via that path.
func TestAPI016_ExternalProjectionSnapshots(t *testing.T) {
	out := evo.Init(evo.Config{
		Format: evo.FormatExternal,
		Stdout: io.Discard,
		Stderr: io.Discard,
	})
	t.Cleanup(func() { _ = out.Close() })
	out.Task("x").Done()
	_ = out.Finish()
	snap := out.Snapshot()
	if len(snap.Tasks) != 1 || snap.Tasks[0].Name != "x" {
		t.Fatalf("expected the task in the snapshot, got %+v", snap.Tasks)
	}
}

const (
	sameRunGroupName    = "packages"
	sameRunTaskFetch    = "curl"
	sameRunTaskStore    = "wget"
	sameRunFactName     = "language"
	sameRunFactValue    = "go"
	sameRunWarn         = "cache miss"
	sameRunEffectVerb   = "write"
	sameRunEffectObject = "binary"
	sameRunEffectQty    = 1
)

// TestProjection_SameRunFactsEffectsConclusion proves one Isolated Init's
// Snapshot is the source of truth for plain stdout, EncodeJSON, and
// EncodeJSONL: Facts, Effects, and Conclusion match across those projections.
func TestProjection_SameRunFactsEffectsConclusion(t *testing.T) {
	var stdout bytes.Buffer
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := evo.Init(evo.Config{
		Isolated:        true,
		Plain:           true,
		Color:           evo.ColorNever,
		Stdout:          &stdout,
		Stderr:          io.Discard,
		Terminal:        screen,
		VisibilityDelay: evo.DelayForTest(0),
	})
	t.Cleanup(func() { _ = out.Close() })

	pkgs := out.Group(sameRunGroupName)
	fetch := pkgs.Task(sameRunTaskFetch)
	store := pkgs.Task(sameRunTaskStore)
	fetch.Record(sameRunEffectVerb, sameRunEffectQty, sameRunEffectObject)
	fetch.Done()
	store.Done()
	out.Fact(sameRunFactName, sameRunFactValue)
	out.Warn(sameRunWarn)
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}

	snap := out.Snapshot()
	if snap.Conclusion == nil {
		t.Fatal("Finish left Snapshot.Conclusion nil")
	}
	assertSameRunSnapshot(t, snap)

	doc, err := evo.EncodeJSON(snap)
	if err != nil {
		t.Fatalf("EncodeJSON: %v", err)
	}
	assertJSONMatchesSnapshot(t, doc, snap)

	raw, err := evo.EncodeJSONL(out.Events())
	if err != nil {
		t.Fatalf("EncodeJSONL: %v", err)
	}
	assertJSONLMatchesSnapshot(t, raw, snap, out.Events())
	assertPlainMatchesSnapshot(t, stdout.String(), snap)
}

func assertSameRunSnapshot(t *testing.T, snap evo.Snapshot) {
	t.Helper()
	if len(snap.Facts) != 1 || snap.Facts[0].Name != sameRunFactName || snap.Facts[0].Value != sameRunFactValue {
		t.Fatalf("snapshot Facts = %+v, want %s=%s", snap.Facts, sameRunFactName, sameRunFactValue)
	}
	conc := snap.Conclusion
	if len(conc.Facts) != 1 || conc.Facts[0] != snap.Facts[0] {
		t.Fatalf("conclusion Facts = %+v, snapshot Facts = %+v", conc.Facts, snap.Facts)
	}
	if len(snap.Changes) != 1 || len(snap.Changes[0].Records) != 1 {
		t.Fatalf("snapshot Changes = %+v, want one Record effect", snap.Changes)
	}
	got := snap.Changes[0]
	rec := got.Records[0]
	if got.Subject != sameRunTaskFetch || rec.Object != sameRunEffectObject || rec.Quantity != sameRunEffectQty {
		t.Fatalf("snapshot effect = %+v, want subject %s object %s qty %d", got, sameRunTaskFetch, sameRunEffectObject, sameRunEffectQty)
	}
	if !conc.Warned {
		t.Fatal("snapshot Conclusion.Warned = false after Warn")
	}
}

func assertJSONMatchesSnapshot(t *testing.T, raw []byte, snap evo.Snapshot) {
	t.Helper()
	var doc evo.JSONDocument
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("EncodeJSON unmarshal: %v\n%s", err, raw)
	}
	conc := snap.Conclusion
	if doc.Conclusion.State != conc.State {
		t.Fatalf("json conclusion.state = %q, snapshot = %q", doc.Conclusion.State, conc.State)
	}
	if doc.Conclusion.Warned != conc.Warned {
		t.Fatalf("json conclusion.warned = %v, snapshot = %v", doc.Conclusion.Warned, conc.Warned)
	}
	if doc.Conclusion.Changed != conc.Changed {
		t.Fatalf("json conclusion.changed = %v, snapshot = %v", doc.Conclusion.Changed, conc.Changed)
	}
	if len(doc.Changes) != len(snap.Changes) {
		t.Fatalf("json changes count = %d, snapshot = %d\n%s", len(doc.Changes), len(snap.Changes), raw)
	}
	for i, ch := range snap.Changes {
		jc := doc.Changes[i]
		if jc.Subject != ch.Subject {
			t.Fatalf("json changes[%d].subject = %q, snapshot = %q", i, jc.Subject, ch.Subject)
		}
		if len(jc.Records) != len(ch.Records) {
			t.Fatalf("json changes[%d] records = %d, snapshot = %d", i, len(jc.Records), len(ch.Records))
		}
		for j, rec := range ch.Records {
			jr := jc.Records[j]
			if jr.Verb != rec.Verb || jr.Object != rec.Object {
				t.Fatalf("json effect = %+v, snapshot = %+v", jr, rec)
			}
			if rec.HasQty && (jr.Quantity == nil || *jr.Quantity != rec.Quantity) {
				t.Fatalf("json effect quantity = %v, snapshot = %d", jr.Quantity, rec.Quantity)
			}
		}
	}
	assertJSONFactsMatchSnapshot(t, raw, snap)
}

func assertJSONFactsMatchSnapshot(t *testing.T, raw []byte, snap evo.Snapshot) {
	t.Helper()
	var tree map[string]any
	if err := json.Unmarshal(raw, &tree); err != nil {
		t.Fatalf("EncodeJSON tree: %v\n%s", err, raw)
	}
	encoded, ok := tree["facts"]
	if !ok {
		return
	}
	got, err := json.Marshal(encoded)
	if err != nil {
		t.Fatalf("facts marshal: %v", err)
	}
	var facts []evo.FactRecord
	if err := json.Unmarshal(got, &facts); err != nil {
		t.Fatalf("facts unmarshal: %v\n%s", err, got)
	}
	if len(facts) != len(snap.Facts) {
		t.Fatalf("json facts = %+v, snapshot = %+v", facts, snap.Facts)
	}
	for i, f := range snap.Facts {
		if facts[i].Name != f.Name || facts[i].Value != f.Value {
			t.Fatalf("json fact[%d] = %+v, snapshot = %+v", i, facts[i], f)
		}
	}
}

func assertJSONLMatchesSnapshot(t *testing.T, raw []byte, snap evo.Snapshot, events []evo.Event) {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) == 0 || (len(lines) == 1 && lines[0] == "") {
		t.Fatal("EncodeJSONL returned no events")
	}
	if len(lines) != len(events) {
		t.Fatalf("EncodeJSONL lines = %d, Events() = %d\n%s", len(lines), len(events), raw)
	}
	runID := ""
	types := make([]string, 0, len(lines))
	var finished evo.EventJSON
	hasEffect := false
	hasFinished := false
	for i, line := range lines {
		var ev evo.EventJSON
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("EncodeJSONL line %d: %v\n%s", i, err, line)
		}
		if ev.Type != events[i].Type {
			t.Fatalf("jsonl[%d] type = %q, Events() = %q", i, ev.Type, events[i].Type)
		}
		if ev.OutputID != "" {
			if runID == "" {
				runID = ev.OutputID
			} else if ev.OutputID != runID {
				t.Fatalf("jsonl output_id drifted: %q vs %q", runID, ev.OutputID)
			}
		}
		types = append(types, ev.Type)
		switch ev.Type {
		case "output.finished":
			hasFinished = true
			finished = ev
		case "change.recorded":
			hasEffect = true
		}
	}
	if runID == "" {
		t.Fatalf("jsonl events missing shared run id, types=%v", types)
	}
	if runID != snap.OutputID {
		t.Fatalf("jsonl run id = %q, snapshot OutputID = %q", runID, snap.OutputID)
	}
	if snap.Conclusion.RunID != "" && runID != snap.Conclusion.RunID {
		t.Fatalf("jsonl run id = %q, conclusion RunID = %q", runID, snap.Conclusion.RunID)
	}
	for _, e := range events {
		if e.OutputID != "" && e.OutputID != runID {
			t.Fatalf("Events() output_id = %q, jsonl run id = %q", e.OutputID, runID)
		}
	}
	if !hasFinished {
		t.Fatalf("jsonl missing output.finished, types=%v", types)
	}
	if finished.State != string(snap.Conclusion.State) {
		t.Fatalf("jsonl output.finished state = %q, snapshot = %q", finished.State, snap.Conclusion.State)
	}
	if !hasEffect {
		t.Fatalf("jsonl missing change.recorded effect event, types=%v", types)
	}
}

func assertPlainMatchesSnapshot(t *testing.T, got string, snap evo.Snapshot) {
	t.Helper()
	if !strings.Contains(got, sameRunTaskFetch) || !strings.Contains(got, sameRunTaskStore) {
		t.Fatalf("plain missing task names %q/%q:\n%s", sameRunTaskFetch, sameRunTaskStore, got)
	}
	for _, f := range snap.Facts {
		if !strings.Contains(got, f.Name) || !strings.Contains(got, f.Value) {
			t.Fatalf("plain missing fact %q=%q:\n%s", f.Name, f.Value, got)
		}
	}
	seen := map[string]struct{}{}
	for _, ch := range snap.Changes {
		seen[ch.Subject] = struct{}{}
		for _, rec := range ch.Records {
			seen[rec.Verb] = struct{}{}
			seen[rec.Object] = struct{}{}
			if !strings.Contains(got, rec.Verb) || !strings.Contains(got, rec.Object) {
				t.Fatalf("plain missing snapshot effect %s %s:\n%s", rec.Verb, rec.Object, got)
			}
		}
	}
	inventedVerbs := []string{"deleted", "created", "removed"}
	for _, invented := range inventedVerbs {
		if _, ok := seen[invented]; ok {
			continue
		}
		if strings.Contains(got, invented) {
			t.Fatalf("plain invented effect %q absent from Snapshot:\n%s", invented, got)
		}
	}
}
