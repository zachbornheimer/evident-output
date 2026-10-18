package guards_test

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/review"
	"github.com/zachbornheimer/evident-output/mcp/v2/internal/docexamples"
)

const (
	contractDocPath = "CONTRACT.md"
	// contractExampleFile is the name each wrapped example is reviewed under.
	contractExampleFile = "contract_example.go"
)

// elidedBody is the "{ ... }" CONTRACT.md writes for a body it leaves to the
// reader; the wrapper replaces it with a body that returns nil.
var elidedBody = regexp.MustCompile(`\{ \.\.\. \}`)

// topLevelFunc marks a block that declares functions, so it is a file body
// rather than a run of statements.
var topLevelFunc = regexp.MustCompile(`(?m)^func `)

// contractExampleHeader is the package clause and imports every reviewed
// example gets; the examples name no other package.
const contractExampleHeader = `package contract

import (
	"context"
	"os"

	evo "github.com/zachbornheimer/evident-output/v2"
)

`

// contractExampleSource is block as a reviewable Go file. A block of
// statements runs inside the run body its prose assumes, with the ctx and r
// the application's run function receives; a block of declarations stands
// as written. Both replace each "{ ... }" with a body that returns nil.
func contractExampleSource(block string) string {
	block = elidedBody.ReplaceAllString(block, "{ return nil }")
	if topLevelFunc.MatchString(block) {
		return contractExampleHeader + block
	}
	return contractExampleHeader +
		"func contractExample(ctx context.Context, r *evo.Run) error {\n" +
		block + "\nreturn nil\n}\n"
}

// TestContractExamplesReviewClean: agents copy CONTRACT.md's code blocks
// the way they copy examples/, so each block must type-check against the
// 2.0 package and draw zero findings from the review the MCP autofixer
// runs on consumer code. A finding here is an anti-pattern the contract
// itself would teach.
func TestContractExamplesReviewClean(t *testing.T) {
	body, err := os.ReadFile(contractDocPath)
	if err != nil {
		t.Fatalf("read %s: %v", contractDocPath, err)
	}
	fences := docexamples.ExtractGoFences(body)
	if len(fences) == 0 {
		t.Fatalf("%s: no go code blocks found", contractDocPath)
	}
	for _, fence := range fences {
		t.Run(fmt.Sprintf("line_%d", fence.Line), func(t *testing.T) {
			src := contractExampleSource(fence.Content)
			apiErrs, err := docexamples.TypeCheckSnippet(elidedBody.ReplaceAllString(fence.Content, "{ return nil }"))
			if err != nil {
				t.Fatalf("%s:%d is not Go: %v\n%s", contractDocPath, fence.Line, err, fence.Content)
			}
			for _, e := range apiErrs {
				t.Errorf("%s:%d type-check: %v", contractDocPath, fence.Line, e)
			}
			res := review.GoSource(contractExampleFile, src)
			for _, f := range res.Findings {
				t.Errorf("%s:%d review %s %s: %s", contractDocPath, fence.Line, f.RuleID, f.Severity, strings.TrimSpace(f.Message))
			}
			if res.RecheckRequired {
				t.Errorf("%s:%d review reports recheck_required=true", contractDocPath, fence.Line)
			}
		})
	}
}
