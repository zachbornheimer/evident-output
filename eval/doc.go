// Package eval is the nested module that will hold the paid, model-backed
// driver for the pit-of-success eval, so model SDKs never enter the library
// module. The deterministic half (task set, grader, reference replay) lives in
// internal/agent/evaltask, which this module may import: Go's internal rule is
// path-based and this module's path sits under the library's.
package eval

import _ "github.com/zachbornheimer/evident-output/internal/agent/evaltask"
