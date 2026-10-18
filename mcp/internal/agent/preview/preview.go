package preview

// Profile is one named terminal profile and the plain-text preview it shows.
type Profile struct {
	Name string `json:"name"`
	Text string `json:"text"`
}

// profileNames are the terminal profiles a preview is reported for.
var profileNames = []string{"wide", "narrow", "ascii-width-60"}

// DefaultProfiles reports plain, the preview a demo run rendered without a
// terminal, once under each common terminal profile.
func DefaultProfiles(plain string) []Profile {
	out := make([]Profile, len(profileNames))
	for i, name := range profileNames {
		out[i] = Profile{Name: name, Text: plain}
	}
	return out
}
