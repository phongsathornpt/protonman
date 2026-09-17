package acp

import (
	"bytes"
	"strings"
	"testing"

	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
)

func TestWriteSessionInfoNotificationUsesCanonicalSessionTitleAndDeduplicates(t *testing.T) {
	sess := &Session{
		id:            "session-info",
		workspaceName: "protonman",
		messages: []model.Message{
			{Role: model.RoleUser, Content: "Implement ACP session info updates"},
		},
	}
	server := &Server{}
	var output bytes.Buffer

	if err := server.writeSessionInfoNotification(&output, sess); err != nil {
		t.Fatalf("writeSessionInfoNotification() error = %v", err)
	}
	got := output.String()
	for _, want := range []string{
		`"method":"session/update"`,
		`"sessionId":"session-info"`,
		`"sessionUpdate":"session_info_update"`,
		`"title":"Implement ACP session info updates"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("session info notification missing %s: %s", want, got)
		}
	}

	before := output.Len()
	if err := server.writeSessionInfoNotification(&output, sess); err != nil {
		t.Fatalf("duplicate writeSessionInfoNotification() error = %v", err)
	}
	if output.Len() != before {
		t.Fatalf("unchanged title emitted another notification: %s", output.String()[before:])
	}

	sess.mu.Lock()
	sess.messages = nil
	sess.mu.Unlock()
	if err := server.writeSessionInfoNotification(&output, sess); err != nil {
		t.Fatalf("fallback writeSessionInfoNotification() error = %v", err)
	}
	if !strings.Contains(output.String()[before:], `"title":"protonman"`) {
		t.Fatalf("fallback title update missing workspace name: %s", output.String()[before:])
	}
}
