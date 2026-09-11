package engine

import (
	"errors"
	"fmt"
	"io"

	"github.com/zachbornheimer/evident-output/internal/render"
)

func writeMachinePresentation(w io.Writer, snap Snapshot, events []Event, proj Projection, misuse error) error {
	var body []byte
	var err error
	switch proj {
	case ProjectionJSON:
		body, err = render.EncodeJSON(snap)
	case ProjectionJSONL:
		body, err = render.EncodeJSONL(events)
	default:
		return misuse
	}
	if err != nil {
		err = fmt.Errorf("%w: %v", ErrRenderer, err)
		if misuse == nil {
			return err
		}
		return errors.Join(misuse, err)
	}
	if w != nil && len(body) > 0 {
		if _, werr := w.Write(body); werr != nil {
			werr = fmt.Errorf("%w: %v", ErrRenderer, werr)
			if misuse == nil {
				return werr
			}
			return errors.Join(misuse, werr)
		}
		if f, ok := w.(flusher); ok {
			_ = f.Flush()
		}
	}
	return misuse
}
