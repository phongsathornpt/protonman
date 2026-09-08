package tui

import (
	"github.com/phongsathornpt/proton/internal/core/permission"
	"testing"
	"time"
)

func TestPermissionRequestLivesInBottomPane(t *testing.T) {
	m := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	m.openPermission(permissionRequest{
		request: permission.Request{
			ToolName: "bash",
			ToolKind: permission.ToolBash,
			Detail:   "pwd",
		},
		response: make(chan permissionResponse, 1),
	})

	if top := m.bottom.top(); top == nil || top.ID() != permissionViewID {
		t.Fatalf("top view = %#v, want permission", top)
	}
	if m.bottom.composerVisible() {
		t.Fatal("composer remained visible while approval view owns input")
	}
}
func TestStalePermissionRequestAfterTurnEndIsDenied(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	response := make(chan permissionResponse, 1)
	updated, _ := model.Update(permissionRequestMsg{request: permissionRequest{
		request:  permission.Request{ToolName: "bash", ToolKind: permission.ToolBash},
		response: response,
	}})
	model = updated.(*bubbleModel)

	if model.hasPermissionView() {
		t.Fatal("stale permission request opened a modal after the turn ended")
	}
	select {
	case result := <-response:
		if result.resolution.Action != permission.ActionDeny {
			t.Fatalf("stale permission action = %s, want deny", result.resolution.Action)
		}
	case <-time.After(time.Second):
		t.Fatal("stale permission request was not resolved")
	}
}
