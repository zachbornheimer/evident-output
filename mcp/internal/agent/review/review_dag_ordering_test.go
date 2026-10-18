package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/review"
)

const (
	afterChainDetector       = "detectAfterChainDuplicatesSequence"
	producerConsumerDetector = "detectMissingProducerConsumerOrdering"
)

func reviewDialectFixture(t *testing.T, detector, name string) review.Result {
	t.Helper()
	return review.GoSource(name, readDialectFixture(t, detector, name))
}

func TestEVODAG002_AfterChainDuplicatesSequence_Fires(t *testing.T) {
	for _, name := range []string{"bad.go", "bad_declared_chain.go", "bad_ambiguous_names.go"} {
		t.Run(name, func(t *testing.T) {
			f := assertFinding(t, reviewDialectFixture(t, afterChainDetector, name), "EVO-DAG-002")
			if !strings.Contains(f.Suggestion, "Sequence") {
				t.Fatalf("suggestion does not name Sequence: %q", f.Suggestion)
			}
		})
	}
}

func TestEVODAG002_NoChain_StaysSilent(t *testing.T) {
	for _, name := range []string{"good.go", "good_single_edge.go", "good_unrelated_type.go"} {
		t.Run(name, func(t *testing.T) {
			assertNoFinding(t, reviewDialectFixture(t, afterChainDetector, name), "EVO-DAG-002")
		})
	}
}

func TestEVODAG003_ProducerConsumerWithNoOrdering_Fires(t *testing.T) {
	for _, name := range []string{"bad.go", "bad_consumer_first.go"} {
		t.Run(name, func(t *testing.T) {
			f := assertFinding(t, reviewDialectFixture(t, producerConsumerDetector, name), "EVO-DAG-003")
			if !strings.Contains(f.Suggestion, "After") || !strings.Contains(f.Suggestion, "Sequence") {
				t.Fatalf("suggestion does not name After and Sequence: %q", f.Suggestion)
			}
		})
	}
}

func TestEVODAG003_OrderedProducerConsumer_StaysSilent(t *testing.T) {
	for _, name := range []string{"good_after.go", "good_sequence.go"} {
		t.Run(name, func(t *testing.T) {
			assertNoFinding(t, reviewDialectFixture(t, producerConsumerDetector, name), "EVO-DAG-003")
		})
	}
}

// The ordering rules need an Evo release that has Sequence and After, so a
// pin older than 1.0.0 gets no finding.
func TestEVODAG002And003_PreOneZeroPin_StaySilent(t *testing.T) {
	const preOneZero = "0.6.0"
	for ruleID, fixture := range map[string][2]string{
		"EVO-DAG-002": {afterChainDetector, "bad.go"},
		"EVO-DAG-003": {producerConsumerDetector, "bad.go"},
	} {
		t.Run(ruleID, func(t *testing.T) {
			res := review.GoSourceAt(fixture[1], readDialectFixture(t, fixture[0], fixture[1]), preOneZero)
			assertNoFinding(t, res, ruleID)
		})
	}
}
