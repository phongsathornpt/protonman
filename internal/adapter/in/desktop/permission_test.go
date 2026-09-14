//go:build desktop

package desktop

import (
	"strings"
	"testing"

	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

func TestFinishPermissionStateIgnoresStaleWaiter(t *testing.T) {
	stale := make(chan string, 1)
	current := make(chan string, 1)
	a := &application{
		permissionWaiters: map[string]chan string{"7": current},
		state: desktopstate.State{
			Sessions:        []desktopstate.SessionState{{ID: "s1", Status: desktopstate.TaskWaitingPermission}},
			PermissionInbox: []desktopstate.PermissionRequest{{RequestID: "7", SessionID: "s1"}},
		},
	}

	if a.finishPermissionState("7", "s1", stale) {
		t.Fatal("stale waiter unexpectedly resolved current permission")
	}
	if got := a.permissionWaiters["7"]; got != current {
		t.Fatal("stale waiter removed replacement waiter")
	}
	if len(a.state.PermissionInbox) != 1 {
		t.Fatalf("permission inbox changed for stale waiter: %#v", a.state.PermissionInbox)
	}
	if got := a.state.Sessions[0].Status; got != desktopstate.TaskWaitingPermission {
		t.Fatalf("session status = %q, want waiting_permission", got)
	}

	if !a.finishPermissionState("7", "s1", current) {
		t.Fatal("current waiter was not resolved")
	}
	if _, ok := a.permissionWaiters["7"]; ok {
		t.Fatal("resolved waiter was not removed")
	}
	if len(a.state.PermissionInbox) != 0 {
		t.Fatalf("permission inbox not cleared: %#v", a.state.PermissionInbox)
	}
	if got := a.state.Sessions[0].Status; got != desktopstate.TaskRunning {
		t.Fatalf("session status = %q, want running", got)
	}
}

func TestPermissionDetailIncludesToolArgumentsAndLocations(t *testing.T) {
	detail := permissionDetail(map[string]any{
		"title": "Run tests",
		"kind":  "execute",
		"rawInput": map[string]any{
			"command": "go test ./...",
		},
		"locations": []map[string]any{{"path": "internal/adapter/in/desktop/permission.go"}},
	})
	for _, want := range []string{"go test ./...", "permission.go", "execute"} {
		if !strings.Contains(detail, want) {
			t.Fatalf("permission detail %q does not contain %q", detail, want)
		}
	}
}

func TestPermissionDetailFailsClosedWhenRuntimeOmitsDetails(t *testing.T) {
	if got := permissionDetail(nil); !strings.Contains(got, "not provided") {
		t.Fatalf("permission detail = %q, want missing-detail warning", got)
	}
}
