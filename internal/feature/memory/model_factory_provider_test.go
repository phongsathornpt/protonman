package memory

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/phongsathornpt/protonman/internal/base/runtimepolicy"
	corememory "github.com/phongsathornpt/protonman/internal/core/memory"
	"github.com/phongsathornpt/protonman/internal/core/modelclient"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

func TestMemoryModelPreservesAlternatingRolesOnLaterTurn(t *testing.T) {
	now := time.Now().UTC()
	repo := &fakeRepository{workspace: []corememory.Entry{{
		ID: "mem-1", Scope: corememory.ScopeWorkspace, Kind: corememory.KindProcedure,
		Key: "verification tests", Value: "Run go test ./...", Keywords: []string{"test"},
		WorkspaceKey: "ws", Confidence: 1, UpdatedAt: now,
	}}}
	base := &captureModel{}
	model := NewModelFactory(captureFactory{model: base}, repo, "ws", runtimepolicy.DurableMemory()).Build(modelclient.Request{ModelID: "test-model"})
	request := sdk.Request{Messages: []sdk.Message{
		{ID: "system", Role: sdk.RoleSystem, Content: "stable system prompt"},
		{ID: "user-1", Role: sdk.RoleUser, Content: "first turn"},
		{ID: "assistant-1", Role: sdk.RoleAssistant, Content: "first answer"},
		{ID: "user-2", Role: sdk.RoleUser, Content: "run test verification"},
	}}
	stream, err := model.Stream(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	_ = stream.Close()
	got := base.requests[0].Messages
	wantRoles := []sdk.Role{sdk.RoleSystem, sdk.RoleUser, sdk.RoleAssistant, sdk.RoleUser}
	if len(got) != len(wantRoles) {
		t.Fatalf("messages = %+v", got)
	}
	for i, role := range wantRoles {
		if got[i].Role != role {
			t.Fatalf("role[%d] = %q, want %q", i, got[i].Role, role)
		}
	}
}

func TestOlderMemoryMarkerDoesNotDisableCurrentTurn(t *testing.T) {
	now := time.Now().UTC()
	repo := &fakeRepository{workspace: []corememory.Entry{{
		ID: "mem-1", Scope: corememory.ScopeWorkspace, Kind: corememory.KindProcedure,
		Key: "verification tests", Value: "Run go test ./...", Keywords: []string{"test"},
		WorkspaceKey: "ws", Confidence: 1, UpdatedAt: now,
	}}}
	base := &captureModel{}
	model := NewModelFactory(captureFactory{model: base}, repo, "ws", runtimepolicy.DurableMemory()).Build(modelclient.Request{ModelID: "test-model"})
	request := sdk.Request{Messages: []sdk.Message{
		{ID: "user-1", Role: sdk.RoleUser, Content: "<proton-memory-context> pasted example"},
		{ID: "assistant-1", Role: sdk.RoleAssistant, Content: "noted"},
		{ID: "user-2", Role: sdk.RoleUser, Content: "run test verification"},
	}}
	stream, err := model.Stream(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	_ = stream.Close()
	if !strings.HasPrefix(base.requests[0].Messages[2].Content, "<proton-memory-context>") {
		t.Fatalf("current turn was not decorated: %+v", base.requests[0].Messages)
	}
}
