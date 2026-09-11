package engine

// MutationOption configures a mutation verb (Create/Update/Delete and siblings).
type MutationOption interface {
	applyMutation(*mutationOpts)
}

type mutationOpts struct {
	quantity int
	hasQty   bool
}

type mutationOptionFunc func(*mutationOpts)

func (f mutationOptionFunc) applyMutation(o *mutationOpts) { f(o) }

// defaultMutationQuantity is the Affected count when the caller names an
// object and omits Affected — one atomic Task, one item.
const defaultMutationQuantity = 1

// Affected sets how many objects one atomic mutation touches. Omit it when
// the Task affects a single item. Affected(0) records nothing; Affected(n<0)
// is misuse.
func Affected(n int) MutationOption {
	return mutationOptionFunc(func(o *mutationOpts) {
		o.quantity = n
		o.hasQty = true
	})
}

func applyMutationOptions(opts []MutationOption) mutationOpts {
	var o mutationOpts
	for _, opt := range opts {
		if opt != nil {
			opt.applyMutation(&o)
		}
	}
	return o
}
