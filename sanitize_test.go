package evo_test

import (
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

func TestSEC001_ItemNameNeutralizesESC(t *testing.T) {
	var buf strings.Builder
	out := evo.New(evo.To(&buf), evo.Plain(), evo.NoColor())
	t.Cleanup(func() { _ = out.Close() })

	out.Item("evil\x1b[31mred").OK()
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "\x1b") {
		t.Fatalf("ESC leaked into plain output:\n%s", buf.String())
	}
}
