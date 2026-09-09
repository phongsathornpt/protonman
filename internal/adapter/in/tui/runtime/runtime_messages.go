package runtime

import (
	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/core/tool"
)

type toolResultMsg struct {
	call   tool.Call
	result tool.Result
	err    error
}

type turnDeltaMsg struct{ event app.Event }

type turnEventsClosedMsg struct{}

type turnDoneMsg struct {
	result app.Result
	err    error
}
