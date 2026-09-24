// Fixture for adopt's interface mutation-facade detection (ZYS-1019).
// homelabctl mutates through an injected facade interface field
// (rt.docker.ComposeUpRecreate, deps.Docker.PullImage) rather than a
// concrete *facade-suffixed type with mutation-verb methods — the shape
// adopt's declaration-based mutation-facade detector cannot see, since
// Docker here is an interface with no method bodies at all.
package mutationinterface

import "strings"

// Docker is the injected facade interface — no method body to inspect,
// only a call site naming a mutation verb (PullImage).
type Docker interface {
	PullImage(image string) error
}

// Deps bundles the interface-typed facade field the way homelabctl's
// runtime bundles its injected dependencies.
type Deps struct {
	Docker Docker
}

// sync calls through the interface field — the exact call site the
// mutation-facade finding must enumerate.
func sync(deps Deps, image string) error {
	return deps.Docker.PullImage(image)
}

// builderIsNotAFacade proves the detector does not flag a concrete,
// externally-defined type's method merely because the name is well known —
// strings.Builder is a concrete struct (not a locally declared interface),
// so WriteString here must NOT be reported as a mutation-facade call.
func builderIsNotAFacade() string {
	var b strings.Builder
	b.WriteString("not a facade")
	return b.String()
}
