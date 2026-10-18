package evo

import (
	"testing"

	"github.com/zachbornheimer/evident-output/v2/testkit/manifesttest"
)

// testProgramName is the binary name every test run reports, so a zero Config.Name resolves to the same
// text whatever the test binary is called.
const testProgramName = "tool"

func TestMain(m *testing.M) {
	processArgv0 = func() string { return testProgramName }
	manifesttest.Main(m)
}
