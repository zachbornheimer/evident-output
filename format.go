package evo

import "github.com/zachbornheimer/evident-output/internal/engine"

func RenderPlain(s Snapshot, opts PlainOptions) ([]byte, error) {
	return engine.RenderPlain(s, opts)
}
