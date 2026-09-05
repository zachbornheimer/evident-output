package typedvalue

// Report reaches evo purely through r.out (declared on Reporter in
// file_a.go) — no evo. qualifier appears anywhere in this file, proving the
// cross-file struct-field resolution the typed-value classifier exists for.
func (r Reporter) Report() {
	r.out.Fact("k", "v")
}

// localLogger shares Fact's name and shape with evo.Output.Fact but has
// nothing to do with evo.
type localLogger struct{}

func (l localLogger) Fact(key, value string) {}

// UsesLocalOut proves the false-positive guard end to end: a local
// variable literally named "out", calling a same-shaped Fact(...) method,
// must not be mistaken for Reporter.out just because the names line up —
// classifyTypedValue only resolves receiver/parameter bindings, never a
// local var, so "out" here is never bound to any type at all.
func UsesLocalOut() {
	var out localLogger
	out.Fact("k", "v")
}
