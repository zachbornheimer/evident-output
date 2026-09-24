package wire

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/wireschema"
)

// releasedV2Documents holds final JSON exactly as the 1.1 release wrote it
// (its goldens, frozen at 3b6c600). The v2 schema keeps one $id, so every
// field added after 1.1 must stay optional: a document a 1.1.x consumer
// already has on disk must still conform to the published v2 schema.
const releasedV2Documents = "testdata/v1.1"

func TestRunSchema_AcceptsReleasedV2Documents(t *testing.T) {
	schema, err := os.ReadFile("../../schema/run.v2.json")
	if err != nil {
		t.Fatal(err)
	}
	paths, err := filepath.Glob(filepath.Join(releasedV2Documents, "*.json"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no released v2 documents under %s (err %v)", releasedV2Documents, err)
	}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			doc, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := wireschema.Validate(schema, doc); err != nil {
				t.Fatalf("a 1.1-written v2 document no longer conforms to schema/run.v2.json:\n%v", err)
			}
		})
	}
}

// releasedRunFinishedPayload is the run.finished payload 1.1 emitted.
const releasedRunFinishedPayload = `{"outcome":"ok","exit_code":0}`

func TestEventSchema_AcceptsReleasedRunFinishedPayload(t *testing.T) {
	schema, err := os.ReadFile("../../schema/event.v2.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := wireschema.ValidateDef(schema, []byte(releasedRunFinishedPayload), "runFinishedPayload"); err != nil {
		t.Fatalf("a 1.1-written run.finished payload no longer conforms to $defs/runFinishedPayload:\n%v", err)
	}
}
