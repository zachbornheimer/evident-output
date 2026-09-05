// Package typedvalue is a golden fixture for the evo-typed-value
// classifier: it proves cross-file struct-field resolution and the
// false-positive guard that keys on owner type, never on bare field or
// variable name. It is deliberately not real production code.
package typedvalue

import (
	evo "github.com/zachbornheimer/evident-output"
)

// Reporter carries an evo-typed field declared here in file_a.go; Report
// (declared in file_b.go) reaches it only through the receiver, with no
// evo. qualifier of its own anywhere in file_b.go — this is the cross-file
// typed-value case the classifier must catch.
type Reporter struct {
	out *evo.Output
}

// Widget's field is also named "out", deliberately, but its type is a plain
// string — proving the classifier keys on Widget's own type, not on the
// bare field name "out" that Reporter happens to share.
type Widget struct {
	out string
}

// Emit touches Widget.out but must never be classified: Widget.out is not
// evo-typed, regardless of the field name matching Reporter.out.
func (w Widget) Emit() string {
	return w.out
}
