package acp

import (
	"io"
	"runtime"
	"sync"
	"weak"

	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/core/session"
)

var sessionInfoTitles sync.Map // map[weak.Pointer[Session]]string

func (s *Server) writeSessionInfoNotification(output io.Writer, sess *Session) error {
	if s == nil || sess == nil || output == nil {
		return nil
	}

	sess.mu.Lock()
	sessionID := sess.id
	workspaceName := sess.workspaceName
	messages := model.SnapshotMessages(sess.messages)
	sess.mu.Unlock()

	preview := session.Preview(session.FromModelMessages(messages))
	title := sessionListTitle(sessionID, workspaceName, preview)
	key := weak.Make(sess)
	if previous, found := sessionInfoTitles.Load(key); found {
		if last, valid := previous.(string); valid && last == title {
			return nil
		}
	} else {
		runtime.AddCleanup(sess, func(key weak.Pointer[Session]) {
			sessionInfoTitles.Delete(key)
		}, key)
	}

	notification := RPCNotification{
		JSONRPC: "2.0",
		Method:  "session/update",
		Params: map[string]any{
			"sessionId": sessionID,
			"update": map[string]any{
				"sessionUpdate": "session_info_update",
				"title":         title,
			},
		},
	}
	if err := WriteJSON(output, &s.writeMu, notification); err != nil {
		return err
	}
	sessionInfoTitles.Store(key, title)
	runtime.KeepAlive(sess)
	return nil
}
