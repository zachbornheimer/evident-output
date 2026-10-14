package evo

import (
	"encoding/json"
	"testing"

	"github.com/zachbornheimer/evident-output/v2/internal/project/wire"
	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// runDocument is the finished run's "evo.run" document, built from its
// conclusion exactly as Finish writes it under FormatJSON.
func (o *Run) runDocument(t testing.TB) []byte {
	t.Helper()
	snap := o.snapshot()
	if snap.Conclusion == nil {
		t.Fatal("run has no conclusion: finish it before reading its evo.run document")
	}
	doc, err := wire.EncodeRun(record.Result{Conclusion: *snap.Conclusion}, wire.Release)
	if err != nil {
		t.Fatalf("encode evo.run: %v", err)
	}
	return doc
}

// decodedRunDocument is runDocument decoded into the wire type.
func (o *Run) decodedRunDocument(t testing.TB) wire.RunDocument {
	t.Helper()
	var doc wire.RunDocument
	if err := json.Unmarshal(o.runDocument(t), &doc); err != nil {
		t.Fatalf("decode evo.run: %v", err)
	}
	return doc
}
