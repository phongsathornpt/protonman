package turn

import (
	"log/slog"

	tea "charm.land/bubbletea/v2"
)

func Wait(events <-chan tea.Msg) tea.Cmd {
	if events == nil {
		return nil
	}
	return func() tea.Msg {
		msg, ok := <-events
		if !ok {
			slog.Debug("tui turn wait observed closed event channel")
			return EventsClosed{}
		}
		return msg
	}
}
