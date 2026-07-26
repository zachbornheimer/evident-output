// Package evo is Evident Output: a presentation library for CLI state, progress,
// evidence, changes, plans, messages, actions, and conclusions.
//
// Application code owns execution. Evo owns presentation.
//
//	out := evo.For("repo")
//	defer out.Close()
//	out.Item("working tree").OK()
//	return out.Finish()
package evo
