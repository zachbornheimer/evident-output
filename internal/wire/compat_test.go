package wire

import (
	"encoding/json"
	"testing"
)

// This file is ZYS-823 gap 4: an older-shape "evo.run" document (written
// before a field existed) must still decode into today's types with no
// loss on the fields it does carry, and a field this package added since
// must be additive — an old consumer type that has never heard of it still
// decodes today's document with no error and no data loss on the fields it
// knows about.
//
// "resource" maps to TrackedResourceDoc and "Patch" maps to VerificationDoc
// (spec §27: Patch reduces to File, and File's own machine truth — per-
// attribute contents/permissions outcome, with the Facts that explain a
// failed one — is VerificationDoc, not a second patch-specific wire type;
// there is no separate Patch JSON type in this package). "Problem" maps to
// ProblemDoc. fixtureResourcePath is an obviously-fake path (never touches
// disk — these tests only exercise json.Marshal/Unmarshal on in-memory
// structs).
const fixtureResourcePath = "fixture://managed/x"

func TestSchemaCompat_ProblemDocOlderShapeDecodesWithNoLoss(t *testing.T) {
	older := []byte(`{"code":"A1","message":"finding","subject":"file.go","detail":"why","count":2,"unit":"line"}`)
	var got ProblemDoc
	if err := json.Unmarshal(older, &got); err != nil {
		t.Fatalf("decode older ProblemDoc shape: %v", err)
	}
	want := ProblemDoc{Code: "A1", Message: "finding", Subject: "file.go", Detail: "why", Count: 2, Unit: "line"}
	if got != want {
		t.Fatalf("older-shape decode = %+v, want %+v (no loss on known fields)", got, want)
	}
	if got.EvidenceTail != "" {
		t.Fatalf("EvidenceTail = %q, want zero value for a fixture that never set it", got.EvidenceTail)
	}
}

func TestSchemaCompat_ProblemDocEvidenceTailIsAdditive(t *testing.T) {
	current := ProblemDoc{Code: "A1", Message: "finding", Detail: "why", EvidenceTail: "tail text"}
	encoded, err := json.Marshal(current)
	if err != nil {
		t.Fatalf("encode current ProblemDoc: %v", err)
	}
	type oldConsumerProblemDoc struct {
		Code    string `json:"code,omitempty"`
		Message string `json:"message,omitempty"`
		Detail  string `json:"detail,omitempty"`
	}
	var old oldConsumerProblemDoc
	if err := json.Unmarshal(encoded, &old); err != nil {
		t.Fatalf("old consumer must decode today's document ignoring evidence_tail: %v", err)
	}
	if old.Code != current.Code || old.Message != current.Message || old.Detail != current.Detail {
		t.Fatalf("old consumer decode = %+v, want fields it knows about preserved", old)
	}
}

func TestSchemaCompat_TrackedResourceDocOlderShapeDecodesWithNoLoss(t *testing.T) {
	older := []byte(`{"kind":"file","path":"` + fixtureResourcePath + `","fingerprint":"sha256:abc"}`)
	var got TrackedResourceDoc
	if err := json.Unmarshal(older, &got); err != nil {
		t.Fatalf("decode older TrackedResourceDoc shape: %v", err)
	}
	want := TrackedResourceDoc{Kind: "file", Path: fixtureResourcePath, Fingerprint: "sha256:abc"}
	if got != want {
		t.Fatalf("older-shape decode = %+v, want %+v", got, want)
	}
}

func TestSchemaCompat_TrackedResourceDocModeIsAdditive(t *testing.T) {
	current := TrackedResourceDoc{Kind: "file", Path: fixtureResourcePath, Mode: "0644"}
	encoded, err := json.Marshal(current)
	if err != nil {
		t.Fatalf("encode current TrackedResourceDoc: %v", err)
	}
	type oldConsumerResourceDoc struct {
		Kind string `json:"kind"`
		Path string `json:"path"`
	}
	var old oldConsumerResourceDoc
	if err := json.Unmarshal(encoded, &old); err != nil {
		t.Fatalf("old consumer must decode today's document ignoring mode: %v", err)
	}
	if old.Kind != current.Kind || old.Path != current.Path {
		t.Fatalf("old consumer decode = %+v, want fields it knows about preserved", old)
	}
}

func TestSchemaCompat_VerificationDocOlderShapeDecodesWithNoLoss(t *testing.T) {
	older := []byte(`{"name":"contents","status":"satisfied"}`)
	var got VerificationDoc
	if err := json.Unmarshal(older, &got); err != nil {
		t.Fatalf("decode older VerificationDoc shape: %v", err)
	}
	want := VerificationDoc{Name: "contents", Status: "satisfied"}
	if got.Name != want.Name || got.Status != want.Status || len(got.Facts) != 0 {
		t.Fatalf("older-shape decode = %+v, want %+v (no loss, no invented Facts)", got, want)
	}
}

func TestSchemaCompat_VerificationDocFactsIsAdditive(t *testing.T) {
	current := VerificationDoc{
		Name: "permissions", Status: "error",
		Facts: []FactDoc{{Name: "error", Value: "operation not permitted"}},
	}
	encoded, err := json.Marshal(current)
	if err != nil {
		t.Fatalf("encode current VerificationDoc: %v", err)
	}
	type oldConsumerVerificationDoc struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	}
	var old oldConsumerVerificationDoc
	if err := json.Unmarshal(encoded, &old); err != nil {
		t.Fatalf("old consumer must decode today's document ignoring facts: %v", err)
	}
	if old.Name != current.Name || old.Status != current.Status {
		t.Fatalf("old consumer decode = %+v, want fields it knows about preserved", old)
	}
}

func TestSchemaCompat_EffectDocOlderShapeDecodesWithNoLoss(t *testing.T) {
	older := []byte(`{"subject":"build","status":"changed","verb":"created","object":"file"}`)
	var got EffectDoc
	if err := json.Unmarshal(older, &got); err != nil {
		t.Fatalf("decode older EffectDoc shape: %v", err)
	}
	want := EffectDoc{Subject: "build", Status: "changed", Verb: "created", Object: "file"}
	if got != want {
		t.Fatalf("older-shape decode = %+v, want %+v", got, want)
	}
	if got.Quantity != nil {
		t.Fatalf("Quantity = %v, want nil for a fixture that never set it", got.Quantity)
	}
}

func TestSchemaCompat_EffectDocQuantityIsAdditive(t *testing.T) {
	qty := int64(3)
	current := EffectDoc{Subject: "build", Status: "changed", Verb: "created", Object: "file", Quantity: &qty}
	encoded, err := json.Marshal(current)
	if err != nil {
		t.Fatalf("encode current EffectDoc: %v", err)
	}
	type oldConsumerEffectDoc struct {
		Subject string `json:"subject"`
		Status  string `json:"status"`
		Verb    string `json:"verb"`
		Object  string `json:"object"`
	}
	var old oldConsumerEffectDoc
	if err := json.Unmarshal(encoded, &old); err != nil {
		t.Fatalf("old consumer must decode today's document ignoring quantity: %v", err)
	}
	if old.Subject != current.Subject || old.Status != current.Status || old.Verb != current.Verb || old.Object != current.Object {
		t.Fatalf("old consumer decode = %+v, want fields it knows about preserved", old)
	}
}
