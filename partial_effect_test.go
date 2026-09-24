package evo_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// refDelete is the aggregate every PartialEffect test requests: delete 3
// remote refs.
var refDelete = evo.EffectSpec{Verb: evo.EffectDelete, Object: "remote ref", Quantity: 3}

// partialResult is what one partialRun observed.
type partialResult struct {
	out       *evo.Output
	rendered  string
	effectErr error
	state     evo.EntityState
}

// partialRun runs one Task whose Define returns Effect(refDelete, fn) in
// the given wire format.
func partialRun(t *testing.T, format evo.Format, fn func(context.Context) error) partialResult {
	t.Helper()
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Title: "prune", Color: evo.ColorNever, Plain: true, Format: format, Stderr: io.Discard})
	t.Cleanup(func() { _ = out.Close() })
	var effectErr error
	task := out.Task("refs")
	task.Define(func(ctx context.Context) error {
		effectErr = evo.Effect(ctx, refDelete, fn)
		return effectErr
	})
	_ = out.Finish()
	return partialResult{out: out, rendered: buf.String(), effectErr: effectErr, state: task.Snapshot().State}
}

// changedQuantities lists every changed Effect record's quantity.
func changedQuantities(snap evo.Snapshot) []int64 {
	var qs []int64
	for _, c := range snap.Changes {
		for _, r := range c.Records {
			qs = append(qs, r.Quantity)
		}
	}
	return qs
}

func TestPartialEffect_TwoOfThreeRecordsCommittedAndFailsTheTask(t *testing.T) {
	t.Parallel()
	cause := errors.New("git push --delete: remote rejected")
	r := partialRun(t, evo.FormatHuman, func(context.Context) error {
		return evo.PartialEffect(2, cause)
	})
	if !errors.Is(r.effectErr, cause) {
		t.Fatalf("Effect err = %v, want the supplied cause", r.effectErr)
	}
	if r.state != evo.Failed {
		t.Fatalf("task state = %v, want Failed", r.state)
	}
	if qs := changedQuantities(r.out.Snapshot()); len(qs) != 1 || qs[0] != 2 {
		t.Fatalf("changed quantities = %v, want exactly [2]", qs)
	}
	if plain := collapse(r.rendered); !strings.Contains(plain, "2 remote refs deleted") || strings.Contains(plain, "3 remote refs") {
		t.Fatalf("human output must report 2 committed, never 3:\n%s", r.rendered)
	}
}

func TestPartialEffect_ZeroOfThreeRecordsNothingAndFails(t *testing.T) {
	t.Parallel()
	cause := errors.New("auth expired")
	r := partialRun(t, evo.FormatHuman, func(context.Context) error {
		return evo.PartialEffect(0, cause)
	})
	if !errors.Is(r.effectErr, cause) || errors.Is(r.effectErr, evo.ErrInvalidPartialEffect) {
		t.Fatalf("Effect err = %v, want the supplied cause and not ErrInvalidPartialEffect", r.effectErr)
	}
	if r.state != evo.Failed {
		t.Fatalf("task state = %v, want Failed", r.state)
	}
	if snap := r.out.Snapshot(); len(snap.Changes) != 0 {
		t.Fatalf("changes = %d, want none for 0 committed", len(snap.Changes))
	}
}

func TestPartialEffect_FullSuccessIsUnchanged(t *testing.T) {
	t.Parallel()
	r := partialRun(t, evo.FormatHuman, func(context.Context) error { return nil })
	if r.effectErr != nil || r.state != evo.Done {
		t.Fatalf("err = %v state = %v, want nil and Done", r.effectErr, r.state)
	}
	if qs := changedQuantities(r.out.Snapshot()); len(qs) != 1 || qs[0] != 3 {
		t.Fatalf("changed quantities = %v, want [3]", qs)
	}
}

func TestPartialEffect_InvalidDataIsRejectedAndRecordsNothing(t *testing.T) {
	t.Parallel()
	cause := errors.New("boom")
	cases := []struct {
		name      string
		committed int
		err       error
		keepCause bool
	}{
		{"committed above requested", 4, cause, true},
		{"negative committed", -1, cause, true},
		{"nil cause", 2, nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := partialRun(t, evo.FormatHuman, func(context.Context) error {
				return evo.PartialEffect(tc.committed, tc.err)
			})
			if !errors.Is(r.effectErr, evo.ErrInvalidPartialEffect) {
				t.Fatalf("Effect err = %v, want ErrInvalidPartialEffect", r.effectErr)
			}
			if tc.keepCause && !errors.Is(r.effectErr, cause) {
				t.Fatalf("Effect err = %v, want the supplied cause still reachable", r.effectErr)
			}
			if r.state != evo.Failed {
				t.Fatalf("task state = %v, want Failed", r.state)
			}
			if snap := r.out.Snapshot(); len(snap.Changes) != 0 {
				t.Fatalf("changes = %d, want no invented partial count", len(snap.Changes))
			}
		})
	}
}

func TestPartialEffect_CauseStaysErrorsIsAndAsCompatible(t *testing.T) {
	t.Parallel()
	cause := &fs.PathError{Op: "unlink", Path: "refs/heads/old", Err: fs.ErrPermission}
	r := partialRun(t, evo.FormatHuman, func(context.Context) error {
		return fmt.Errorf("deleting refs: %w", evo.PartialEffect(1, cause))
	})
	var pathErr *fs.PathError
	if !errors.As(r.effectErr, &pathErr) || pathErr != cause {
		t.Fatalf("errors.As(%v, *fs.PathError) failed", r.effectErr)
	}
	if !errors.Is(r.effectErr, fs.ErrPermission) {
		t.Fatalf("errors.Is(%v, fs.ErrPermission) = false", r.effectErr)
	}
}

func TestPartialEffect_ErrorTextIsTheCause(t *testing.T) {
	t.Parallel()
	cause := errors.New("remote rejected")
	if got := evo.PartialEffect(2, cause).Error(); !strings.Contains(got, "remote rejected") {
		t.Fatalf("PartialEffect error text = %q, want it to carry the cause", got)
	}
}

func TestPartialEffect_MachineProjectionsAgreeOnCommittedCount(t *testing.T) {
	t.Parallel()
	partial := func(context.Context) error { return evo.PartialEffect(2, errors.New("rejected")) }

	t.Run("snapshot JSON", func(t *testing.T) {
		t.Parallel()
		r := partialRun(t, evo.FormatHuman, partial)
		data, err := evo.EncodeJSON(r.out.Snapshot())
		if err != nil {
			t.Fatal(err)
		}
		var doc evo.JSONDocument
		if err := json.Unmarshal(data, &doc); err != nil {
			t.Fatal(err)
		}
		if len(doc.Changes) != 1 || len(doc.Changes[0].Records) != 1 || *doc.Changes[0].Records[0].Quantity != 2 {
			t.Fatalf("JSON changes = %s, want one record with quantity 2", data)
		}
	})

	t.Run("final JSON document", func(t *testing.T) {
		t.Parallel()
		r := partialRun(t, evo.FormatJSON, partial)
		var doc struct {
			Data struct {
				Effects []struct {
					Status   string `json:"status"`
					Quantity *int64 `json:"quantity"`
				} `json:"effects"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(r.rendered), &doc); err != nil {
			t.Fatalf("decode %q: %v", r.rendered, err)
		}
		if len(doc.Data.Effects) != 1 || doc.Data.Effects[0].Quantity == nil || *doc.Data.Effects[0].Quantity != 2 {
			t.Fatalf("final JSON effects = %s, want one changed effect with quantity 2", r.rendered)
		}
	})

	t.Run("JSONL stream", func(t *testing.T) {
		t.Parallel()
		r := partialRun(t, evo.FormatJSONL, partial)
		committed := 0
		for line := range strings.SplitSeq(strings.TrimSpace(r.rendered), "\n") {
			var ev struct {
				Type    string         `json:"type"`
				Payload map[string]any `json:"payload"`
			}
			if err := json.Unmarshal([]byte(line), &ev); err != nil {
				t.Fatalf("decode %q: %v", line, err)
			}
			if ev.Type != "effect.committed" {
				continue
			}
			committed++
			if q, _ := ev.Payload["quantity"].(float64); q != 2 {
				t.Fatalf("effect.committed payload = %v, want quantity 2", ev.Payload)
			}
		}
		if committed != 1 {
			t.Fatalf("effect.committed events = %d, want 1:\n%s", committed, r.rendered)
		}
	})
}
