package review

import "path/filepath"

// DetectorUnderTest is one registered file detector as the external test
// package sees it: its name (the fixture directory), whether its fixtures are
// dirty 1.x input, and a way to run just that detector over a source file.
type DetectorUnderTest struct {
	Name      string
	Migration bool
	Run       func(path, src, pin string) ([]Finding, error)
}

// DetectorsUnderTest lists every registered file detector.
func DetectorsUnderTest() []DetectorUnderTest {
	out := make([]DetectorUnderTest, 0, len(fileDetectors))
	for _, d := range fileDetectors {
		out = append(out, DetectorUnderTest{Name: d.name, Migration: d.migration, Run: func(path, src, pin string) ([]Finding, error) {
			fset, f, err := parseSourceFile(filepath.Base(path), src)
			if err != nil {
				return nil, err
			}
			in := fileInput{filename: filepath.Base(path), src: src, desiredVersion: pin, file: f, fset: fset, hasEvo: evoImportName(f) != ""}
			if !d.admits(in) {
				return nil, nil
			}
			return admitDialect(d.run(in), pin), nil
		}})
	}
	return out
}
