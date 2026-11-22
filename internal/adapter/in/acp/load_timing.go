package acp

import (
	"encoding/json"
	"time"

	"github.com/phongsathornpt/protonman/internal/base/envconfig"
)

// loadTimings records where a session/load request spends its time. It is
// diagnostic metadata: when PROTONMAN_TIMING is enabled the values ride the ACP
// `_meta` extension bag on the response, so a native client can attribute load
// latency without changing protocol behavior for other agents.
type loadTimings struct {
	newSession     time.Duration
	diskLoad       time.Duration
	restore        time.Duration
	replay         time.Duration
	replayMessages int
	configOptions  time.Duration
	sessionLoad    time.Duration
	total          time.Duration
}

// timingEnabled reports whether diagnostic stage timings are requested.
func timingEnabled() bool {
	return envconfig.Bool(envconfig.Timing)
}

// meta encodes the timings under `_meta.protonman.timings` using microseconds.
func (t loadTimings) meta() Meta {
	payload, err := json.Marshal(map[string]any{
		"timings": map[string]any{
			"newSessionUs":    t.newSession.Microseconds(),
			"diskLoadUs":      t.diskLoad.Microseconds(),
			"restoreUs":       t.restore.Microseconds(),
			"replayUs":        t.replay.Microseconds(),
			"replayMessages":  t.replayMessages,
			"configOptionsUs": t.configOptions.Microseconds(),
			"sessionLoadUs":   t.sessionLoad.Microseconds(),
			"totalUs":         t.total.Microseconds(),
		},
	})
	if err != nil {
		return nil
	}
	return Meta{"protonman": payload}
}
