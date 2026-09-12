package transientnotice

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

const DefaultDuration = 2 * time.Second

type Expired struct{ ID uint64 }

func ExpireAfter(id uint64, duration time.Duration) tea.Cmd {
	if duration <= 0 {
		duration = DefaultDuration
	}
	return func() tea.Msg {
		timer := time.NewTimer(duration)
		defer timer.Stop()
		<-timer.C
		return Expired{ID: id}
	}
}
