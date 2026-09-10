package turn

import "github.com/phongsathornpt/protonman/internal/app"

type Delta struct {
	Event app.Event
}

type EventsClosed struct{}

type Done struct {
	Result app.Result
	Err    error
}
