package preview

import (
	"github.com/zachbornheimer/evident-output/internal/project"
	"github.com/zachbornheimer/evident-output/internal/project/plain"
	"github.com/zachbornheimer/evident-output/internal/record"
)

// Profile is one terminal profile rendering.
type Profile struct {
	Name string `json:"name"`
	Text string `json:"text"`
}

// DefaultProfiles returns plain previews for common capability profiles.
func DefaultProfiles(snap record.Snapshot) []Profile {
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
		text := plain.Plain(snap, s.width, true, false, project.GlyphsUnicode)
		out = append(out, Profile{Name: s.name, Text: text})
	}
	return out
}
