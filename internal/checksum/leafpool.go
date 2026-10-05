package checksum

import (
	"context"
	"sync"
)

// leafPool runs leaf digests with bounded parallelism and keeps the first
// failure, cancelling the rest of the walk when one occurs.
type leafPool struct {
	slots  chan struct{}
	wg     sync.WaitGroup
	mu     sync.Mutex
	err    error
	cancel context.CancelCauseFunc
}

func newLeafPool(parallelism int, cancel context.CancelCauseFunc) *leafPool {
	return &leafPool{slots: make(chan struct{}, parallelism), cancel: cancel}
}

// digest schedules fn and stores its result in n.digest. It blocks while
// every slot is busy, so a huge tree never holds more than parallelism
// files open. A cancelled walk schedules nothing more.
func (p *leafPool) digest(ctx context.Context, n *node, fn func(context.Context) (Digest, error)) {
	select {
	case p.slots <- struct{}{}:
	case <-ctx.Done():
		return
	}
	p.wg.Go(func() {
		defer func() { <-p.slots }()
		d, err := fn(ctx)
		if err != nil {
			p.fail(err)
			return
		}
		n.digest = d
	})
}

func (p *leafPool) fail(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.err == nil {
		p.err = err
		p.cancel(err)
	}
}

// wait returns once every scheduled digest finished, with the first error.
func (p *leafPool) wait() error {
	p.wg.Wait()
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.err
}
