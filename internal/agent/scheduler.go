package agent

import "context"

func (c *Coordinator) acquireWorkspace(ctx context.Context, exclusive bool) (func(), error) {
	count := 1
	writerHeld := false
	if exclusive {
		select {
		case c.wsWriter <- struct{}{}:
			writerHeld = true
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		count = cap(c.wsGate)
	}
	acquired := 0
	for acquired < count {
		select {
		case c.wsGate <- struct{}{}:
			acquired++
		case <-ctx.Done():
			for acquired > 0 {
				<-c.wsGate
				acquired--
			}
			if writerHeld {
				<-c.wsWriter
			}
			return nil, ctx.Err()
		}
	}
	return func() {
		for released := 0; released < count; released++ {
			<-c.wsGate
		}
		if writerHeld {
			<-c.wsWriter
		}
	}, nil
}
