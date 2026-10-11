package change

import (
	"context"
	"errors"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/graph"
)

func deleteTwoBranches() EffectSpec {
	return EffectSpec{Verb: EffectDelete, Object: "branch", Quantity: 2}
}

func TestEffectOutsideADefineCallbackReturnsErrNoTaskContext(t *testing.T) {
	err := Effect(context.Background(), deleteTwoBranches(), func(context.Context) error { return nil })

	if !errors.Is(err, graph.ErrNoTaskContext) {
		t.Fatalf("Effect without a scope = %v, want ErrNoTaskContext", err)
	}
}

func TestEffectRefusesAContentFreeSpecWithoutRunningTheCallback(t *testing.T) {
	cases := map[string]struct {
		spec EffectSpec
		want error
	}{
		"unknown verb":    {EffectSpec{Verb: "write", Object: "file", Quantity: 1}, ErrEffectVerbInvalid},
		"blank object":    {EffectSpec{Verb: EffectDelete, Object: " ", Quantity: 1}, ErrEffectObjectMissing},
		"zero quantity":   {EffectSpec{Verb: EffectDelete, Object: "branch"}, ErrEffectQuantityNotPositive},
		"negative amount": {EffectSpec{Verb: EffectDelete, Object: "branch", Quantity: -1}, ErrEffectQuantityNotPositive},
	}
	for name, tc := range cases {
		host := &fakeHost{applies: true}
		_, ctx := scopedRun(t, host)
		ran := false

		err := Effect(ctx, tc.spec, func(context.Context) error { ran = true; return nil })

		if !errors.Is(err, tc.want) || ran || len(host.recorded()) != 0 {
			t.Errorf("%s: err = %v, ran = %v, entries = %d; want %v, no run, no entry", name, err, ran, len(host.recorded()), tc.want)
		}
	}
	_, ctx := scopedRun(t, &fakeHost{applies: true})
	if err := Effect(ctx, deleteTwoBranches(), nil); !errors.Is(err, ErrEffectCallbackMissing) {
		t.Errorf("nil callback: err = %v, want ErrEffectCallbackMissing", err)
	}
}

func TestEffectInAPlannedRunRecordsOneEntryAndNeverRunsTheCallback(t *testing.T) {
	host := &fakeHost{applies: false}
	_, ctx := scopedRun(t, host)
	ran := false

	err := Effect(ctx, deleteTwoBranches(), func(context.Context) error { ran = true; return nil })

	if err != nil || ran {
		t.Fatalf("planned Effect: err = %v, ran = %v; want nil and not run", err, ran)
	}
	if got := host.recorded(); len(got) != 1 || got[0].Verb() != "delete" {
		t.Fatalf("planned Effect recorded %v, want one delete entry", got)
	}
}

func TestEffectInAnAppliedRunRecordsTheEntryOnlyWhenTheCallbackSucceeds(t *testing.T) {
	host := &fakeHost{applies: true}
	_, ctx := scopedRun(t, host)

	if err := Effect(ctx, deleteTwoBranches(), func(context.Context) error { return nil }); err != nil {
		t.Fatalf("Effect = %v, want nil", err)
	}
	boom := errors.New("boom")
	if err := Effect(ctx, deleteTwoBranches(), func(context.Context) error { return boom }); err != boom {
		t.Fatalf("failing Effect returned %v, want the callback's own error unchanged", err)
	}

	if got := host.recorded(); len(got) != 1 {
		t.Fatalf("recorded %d entries, want 1 (only the success)", len(got))
	}
}

func TestEffectRecordsTheCommittedSubsetOfAPartialEffect(t *testing.T) {
	host := &fakeHost{applies: true}
	_, ctx := scopedRun(t, host)
	cause := errors.New("remote refused")

	err := Effect(ctx, deleteTwoBranches(), func(context.Context) error { return PartialEffect(1, cause) })

	if !errors.Is(err, cause) {
		t.Fatalf("Effect = %v, want it to read as its cause", err)
	}
	got := host.recorded()
	if len(got) != 1 || got[0].Payload("delete")["quantity"] != 1 {
		t.Fatalf("recorded %v, want one entry counting the 1 committed", got)
	}
}

func TestEffectRejectsAPartialEffectThatCommittedMoreThanRequested(t *testing.T) {
	host := &fakeHost{applies: true}
	_, ctx := scopedRun(t, host)

	err := Effect(ctx, deleteTwoBranches(), func(context.Context) error { return PartialEffect(3, errors.New("x")) })

	if !errors.Is(err, ErrInvalidPartialEffect) || len(host.recorded()) != 0 {
		t.Fatalf("err = %v, entries = %d; want ErrInvalidPartialEffect and no entry", err, len(host.recorded()))
	}
}

func TestEffectWhoseTaskDisownsItsWorkRecordsNothing(t *testing.T) {
	host := &fakeHost{applies: true}
	run, ctx := scopedRun(t, host)

	err := Effect(ctx, deleteTwoBranches(), func(context.Context) error {
		if !run.EffectInFlight("t1") {
			t.Error("EffectInFlight(t1) = false while the callback runs")
		}
		run.DenyEffect("t1")
		return nil
	})

	if err != nil || len(host.recorded()) != 0 {
		t.Fatalf("disowned Effect: err = %v, entries = %d; want nil and none", err, len(host.recorded()))
	}
	if run.EffectInFlight("t1") {
		t.Error("EffectInFlight(t1) = true after the callback returned")
	}
}

func TestEffectInFlightForOneTaskDisownsOnlyThatTask(t *testing.T) {
	host := &fakeHost{applies: true}
	run, ctx := scopedRun(t, host)

	err := Effect(ctx, deleteTwoBranches(), func(context.Context) error {
		run.DenyEffect("another")
		return nil
	})

	if err != nil || len(host.recorded()) != 1 {
		t.Fatalf("err = %v, entries = %d; want nil and the entry kept", err, len(host.recorded()))
	}
}

func TestEffectReturnsTheLedgerRefusal(t *testing.T) {
	refusal := errors.New("task already resolved")
	_, ctx := scopedRun(t, &fakeHost{ledgerErr: refusal})
	ran := false

	err := Effect(ctx, deleteTwoBranches(), func(context.Context) error { ran = true; return nil })

	if !errors.Is(err, refusal) || ran {
		t.Fatalf("err = %v, ran = %v; want the ledger's refusal and no run", err, ran)
	}
}

func TestEffectDryRunRejectsAnInvalidResourceLikeAnAppliedRun(t *testing.T) {
	_, ctx := scopedRun(t, &fakeHost{applies: false})
	spec := deleteTwoBranches()
	spec.Resource = LogicalResource("")

	err := Effect(ctx, spec, func(context.Context) error { return nil })

	if !errors.Is(err, ErrInvalidResource) {
		t.Fatalf("planned Effect with an empty resource = %v, want ErrInvalidResource", err)
	}
}

func TestEffectHoldsItsResourceWhileTheCallbackRuns(t *testing.T) {
	_, ctx := scopedRun(t, &fakeHost{applies: true})
	spec := deleteTwoBranches()
	spec.Resource = LogicalResource("remote")

	err := Effect(ctx, spec, func(held context.Context) error {
		return CheckClaimFree(held, "write other")
	})

	if !errors.Is(err, ErrNestedResourceAcquisition) {
		t.Fatalf("callback under a resource: CheckClaimFree = %v, want ErrNestedResourceAcquisition", err)
	}
}

func TestEffectFlightsDeleteTheirTaskOnceNothingIsInFlight(t *testing.T) {
	var flights effectFlights
	first := flights.enter("t1")
	second := flights.enter("t1")
	flights.deny("t1")

	if !flights.exit("t1", second) || !flights.exit("t1", first) {
		t.Fatal("a denial while both flights were up must disown both")
	}
	if flights.active("t1") || len(flights.inFlight) != 0 || len(flights.denials) != 0 {
		t.Errorf("flights = %v / %v, want nothing kept for t1", flights.inFlight, flights.denials)
	}
	flights.deny("t1")
	if flights.exit("t1", flights.enter("t1")) {
		t.Error("a denial with nothing in flight must not disown a later flight")
	}
}
