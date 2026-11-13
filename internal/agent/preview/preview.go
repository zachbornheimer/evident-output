// Package preview generates multi-profile plain previews from snapshots.
package preview

import (
	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/internal/render"
)

// Profile is one terminal profile rendering.
type Profile struct {
	Name string `json:"name"`
	Text string `json:"text"`
}

// DefaultProfiles returns plain previews for common capability profiles.
func DefaultProfiles(snap evo.Snapshot) []Profile {
	specs := []struct {
		name  string
		width int
	}{
		{"wide", 80},
		{"narrow", 30},
		{"ascii-width-60", 60},
	}
	var out []Profile
	for _, s := range specs {
		text := render.Plain(snap, s.width, true, false, evo.GlyphsUnicode)
		out = append(out, Profile{Name: s.name, Text: text})
	}
	return out
}
