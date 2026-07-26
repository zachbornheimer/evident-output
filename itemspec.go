package evo

// ItemSpec is advanced item construction (§11.3).
type ItemSpec struct {
	Key         string
	Name        string
	Description string
	Order       int
	Hidden      bool
	ManualStart bool
}
