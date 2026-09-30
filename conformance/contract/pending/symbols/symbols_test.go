//go:build evopending

// Pending: ZYS-1202 yaegi symbols package shipped by Evo. Red today because
// the symbols directory does not exist. GUESSED PATH: <module>/symbols, with
// generated Go files whose symbol table keys every exported package-level
// function as a quoted name. The test reads the source instead of importing
// it, so `go mod tidy` keeps working before the package exists. The
// stale-check asserts every exported function in the frozen API golden has a
// symbol entry.
package symbols_test

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	apiGolden  = "../../../../testdata/api_golden.txt"
	symbolsDir = "../../../../symbols"
	funcPrefix = "func "
)

func readSymbolSources(t *testing.T) string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(symbolsDir, "*.go"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no symbols package at %s (glob err %v)", symbolsDir, err)
	}
	var sb strings.Builder
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		sb.Write(data)
	}
	return sb.String()
}

func TestSymbolsCoverEveryExportedPackageFunction(t *testing.T) {
	source := readSymbolSources(t)
	f, err := os.Open(apiGolden)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, funcPrefix) || strings.HasPrefix(line, funcPrefix+"(") {
			continue
		}
		name, _, _ := strings.Cut(strings.TrimPrefix(line, funcPrefix), "(")
		if !strings.Contains(source, `"`+name+`"`) {
			t.Errorf("symbols is stale: missing %s", name)
		}
	}
}
