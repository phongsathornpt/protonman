package turn

import (
	"runtime"
	"sync"
	"weak"

	"github.com/phongsathornpt/protonman/proton-sdk/domain"
	"github.com/phongsathornpt/protonman/proton-sdk/usecase"
)

type loopUsageState struct {
	Usage      domain.Usage
	Generation uint64
}

var loopUsageStates sync.Map // map[weak.Pointer[Loop]]loopUsageState

func recordModelUsage(loop *Loop, usage domain.Usage) {
	if loop == nil {
		return
	}
	key := weak.Make(loop)
	generation := uint64(1)
	if previous, ok := loopUsageStates.Load(key); ok {
		if state, ok := previous.(loopUsageState); ok {
			generation = state.Generation + 1
		}
	} else {
		runtime.AddCleanup(loop, func(key weak.Pointer[Loop]) {
			loopUsageStates.Delete(key)
		}, key)
	}
	loopUsageStates.Store(key, loopUsageState{Usage: usage, Generation: generation})
	runtime.KeepAlive(loop)
}

// ContextUsage returns the most recent provider-reported context usage for this
// loop together with the model's effective context-window size. Generation is
// incremented for every fresh provider usage event so clients can avoid replaying
// stale usage after cancellations or reconnect bookkeeping.
func (l *Loop) ContextUsage() (used int64, size int64, generation uint64, ok bool) {
	if l == nil || l.languageModel == nil {
		return 0, 0, 0, false
	}
	value, found := loopUsageStates.Load(weak.Make(l))
	if !found {
		return 0, 0, 0, false
	}
	state, valid := value.(loopUsageState)
	if !valid || state.Generation == 0 {
		return 0, 0, 0, false
	}
	contextWindow := usecase.ModelContextWindow(l.languageModel)
	if contextWindow <= 0 {
		return 0, 0, 0, false
	}
	used = state.Usage.TotalTokens
	if used <= 0 {
		used = state.Usage.InputTokens + state.Usage.OutputTokens
	}
	if used < 0 {
		return 0, 0, 0, false
	}
	return used, int64(contextWindow), state.Generation, true
}
