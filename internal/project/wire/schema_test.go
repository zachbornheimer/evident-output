package wire

import (
	"bytes"
	"os"
	"testing"
)

func TestCheckedInSchemasMatchGenerated(t *testing.T) {
	t.Parallel()
	cases := []struct {
		path string
		got  []byte
	}{
		{"../../../schema/run.v3.json", RunJSONSchema()},
		{"../../../schema/event.v2.json", EventJSONSchema()},
	}
	for _, c := range cases {
		want, err := os.ReadFile(c.path)
		if err != nil {
			t.Errorf("read %s: %v (run go run ./tools/genschema from the module root)", c.path, err)
			continue
		}
		if !bytes.Equal(want, c.got) {
			t.Errorf("%s differs from the schema generated from Go wire types; run go run ./tools/genschema", c.path)
		}
	}
}
