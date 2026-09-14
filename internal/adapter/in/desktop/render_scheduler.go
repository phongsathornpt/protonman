//go:build desktop

package desktop

import (
	"sync"
	"time"
)

const transcriptRenderInterval = 40 * time.Millisecond

type transcriptRenderScheduler struct {
	mu        sync.Mutex
	scheduled bool
}

var transcriptRenderSchedulers sync.Map // map[*application]*transcriptRenderScheduler

func transcriptSchedulerFor(a *application) *transcriptRenderScheduler {
	if existing, ok := transcriptRenderSchedulers.Load(a); ok {
		return existing.(*transcriptRenderScheduler)
	}
	created := &transcriptRenderScheduler{}
	actual, _ := transcriptRenderSchedulers.LoadOrStore(a, created)
	return actual.(*transcriptRenderScheduler)
}

// scheduleTranscriptRender coalesces token-sized ACP chunks into one UI render
// per frame-sized interval. The transcript remains append-only, but Markdown is
// no longer reparsed for every individual model delta.
func (a *application) scheduleTranscriptRender() {
	scheduler := transcriptSchedulerFor(a)
	scheduler.mu.Lock()
	if scheduler.scheduled {
		scheduler.mu.Unlock()
		return
	}
	scheduler.scheduled = true
	scheduler.mu.Unlock()

	time.AfterFunc(transcriptRenderInterval, func() {
		scheduler.mu.Lock()
		scheduler.scheduled = false
		scheduler.mu.Unlock()
		a.renderActiveView()
	})
}
