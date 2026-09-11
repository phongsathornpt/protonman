package runtime

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

const transientNoticeDuration = 2 * time.Second

type transientNoticeExpiredMsg struct{ id uint64 }

func (m *bubbleModel) showTransientNotice(text string) tea.Cmd {
	if m == nil {
		return nil
	}
	m.transientNoticeID++
	id := m.transientNoticeID
	m.transientNotice = strings.TrimSpace(text)
	m.refreshFrameChromeOnly()
	return func() tea.Msg {
		timer := time.NewTimer(transientNoticeDuration)
		defer timer.Stop()
		<-timer.C
		return transientNoticeExpiredMsg{id: id}
	}
}
