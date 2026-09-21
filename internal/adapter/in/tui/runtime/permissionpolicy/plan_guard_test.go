package permissionpolicy

import (
	"context"
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/permission"
)

func TestEvaluatePlanModeCall(t *testing.T) {
	tests := []struct {
		name    string
		request permission.Request
		wantErr bool
	}{
		{
			name: "read tool allowed",
			request: permission.Request{
				ToolName: "read",
				ToolKind: permission.ToolRead,
			},
			wantErr: false,
		},
		{
			name: "grep tool allowed",
			request: permission.Request{
				ToolName: "grep",
				ToolKind: permission.ToolGrep,
			},
			wantErr: false,
		},
		{
			name: "task tool allowed",
			request: permission.Request{
				ToolName: "todo",
				ToolKind: permission.ToolTask,
			},
			wantErr: false,
		},
		{
			name: "read-only bash allowed",
			request: permission.Request{
				ToolName:  "bash",
				ToolKind:  permission.ToolBash,
				Arguments: []byte(`{"command":"ls -la"}`),
			},
			wantErr: false,
		},
		{
			name: "mutating bash blocked",
			request: permission.Request{
				ToolName:  "bash",
				ToolKind:  permission.ToolBash,
				Arguments: []byte(`{"command":"rm -rf /tmp/foo"}`),
			},
			wantErr: true,
		},
		{
			name: "write tool blocked",
			request: permission.Request{
				ToolName: "write",
				ToolKind: permission.ToolEdit,
			},
			wantErr: true,
		},
		{
			name: "subagent get allowed",
			request: permission.Request{
				ToolName:  "subagent",
				ToolKind:  permission.ToolAgent,
				Arguments: []byte(`{"action":"get"}`),
			},
			wantErr: false,
		},
		{
			name: "subagent spawn blocked",
			request: permission.Request{
				ToolName:  "subagent",
				ToolKind:  permission.ToolAgent,
				Arguments: []byte(`{"action":"spawn"}`),
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := EvaluatePlanModeCall(tc.request)
			if (err != nil) != tc.wantErr {
				t.Fatalf("EvaluatePlanModeCall() err = %v, wantErr = %v", err, tc.wantErr)
			}
		})
	}
}

func TestNewPlanModeGuard(t *testing.T) {
	active := false
	guard := NewPlanModeGuard(func() bool { return active })

	req := permission.Request{ToolName: "write", ToolKind: permission.ToolEdit}

	// Inactive
	if err := guard(context.Background(), req); err != nil {
		t.Fatalf("expected nil when inactive, got %v", err)
	}

	// Active
	active = true
	if err := guard(context.Background(), req); err == nil {
		t.Fatal("expected error when active, got nil")
	}
}
