package agent

import "context"

func (c *Coordinator) acquireWorkspace(ctx context.Context, exclusive bool) (func(), error) {
	writerHeld := false
	if exclusive {
		select {
		case c.wsWriter <- struct{}{}:
			writerHeld = true
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	// Admission is held only while acquiring workspace capacity. Once a writer
	// owns it, later readers cannot continuously jump ahead while existing
	// readers drain. Readers still execute concurrently after admission.
	select {
	case c.wsAdmission <- struct{}{}:
	case <-ctx.Done():
		if writerHeld {
			<-c.wsWriter
		}
		return nil, ctx.Err()
	}
	admissionHeld := true
	releaseAdmission := func() {
		if admissionHeld {
			<-c.wsAdmission
			admissionHeld = false
		}
	}

	count := 1
	if exclusive {
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
			releaseAdmission()
			if writerHeld {
				<-c.wsWriter
			}
			return nil, ctx.Err()
		}
	}
	releaseAdmission()

	return func() {
		for released := 0; released < count; released++ {
			<-c.wsGate
		}
		if writerHeld {
			<-c.wsWriter
		}
	}, nil
}
