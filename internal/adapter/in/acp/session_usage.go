package acp

import (
	"io"
	"runtime"
	"sync"
	"weak"
)

type contextUsageProvider interface {
	ContextUsage() (used int64, size int64, generation uint64, ok bool)
}

var sessionUsageGenerations sync.Map // map[weak.Pointer[Session]]uint64

func (s *Server) writeSessionUsageNotification(output io.Writer, sess *Session) error {
	if s == nil || sess == nil || output == nil {
		return nil
	}

	sess.mu.Lock()
	runner := sess.runner
	sessionID := sess.id
	sess.mu.Unlock()

	provider, ok := runner.(contextUsageProvider)
	if !ok || provider == nil {
		return nil
	}
	used, size, generation, ok := provider.ContextUsage()
	if !ok || used < 0 || size <= 0 || generation == 0 {
		return nil
	}

	key := weak.Make(sess)
	if previous, found := sessionUsageGenerations.Load(key); found {
		if last, valid := previous.(uint64); valid && generation <= last {
			return nil
		}
	} else {
		runtime.AddCleanup(sess, func(key weak.Pointer[Session]) {
			sessionUsageGenerations.Delete(key)
		}, key)
	}

	notification := RPCNotification{
		JSONRPC: "2.0",
		Method:  "session/update",
		Params: map[string]any{
			"sessionId": sessionID,
			"update": map[string]any{
				"sessionUpdate": "usage_update",
				"used":          uint64(used),
				"size":          uint64(size),
			},
		},
	}
	if err := WriteJSON(output, &s.writeMu, notification); err != nil {
		return err
	}
	sessionUsageGenerations.Store(key, generation)
	runtime.KeepAlive(sess)
	return nil
}
