package e2e_test

import (
	"context"
	"testing"

	"github.com/phongsathornpt/proton/internal/adapter/out/model"
	agenttool "github.com/phongsathornpt/proton/internal/adapter/out/tool/agent"
	"github.com/phongsathornpt/proton/internal/adapter/out/tool/builtin"
	"github.com/phongsathornpt/proton/internal/app"
	"github.com/phongsathornpt/proton/internal/core/permission"
	"github.com/phongsathornpt/proton/internal/engine/toolcall"
	"github.com/phongsathornpt/proton/internal/feature/agent"
)

func TestE2ESubagentCapabilityChangesPublishedToolsBetweenTurns(t *testing.T) {
	server := newMockLLMServer(t)
	coord := agent.NewCoordinator(nil, nil, nil, nil)
	defer coord.Close()

	base, err := builtin.NewRegistry(
		agenttool.NewDelegateTask(coord),
		agenttool.NewWaitAgent(coord),
		agenttool.NewGetAgent(coord),
		agenttool.NewListAgents(coord),
		agenttool.NewCancelAgent(coord),
	)
	if err != nil {
		t.Fatal(err)
	}
	registry := agenttool.NewCapabilityRegistry(base, coord)
	policy, err := permission.NewPolicy(permission.Config{})
	if err != nil {
		t.Fatal(err)
	}
	service, err := toolcall.NewService(registry, policy, toolcall.WithMode(permission.ModeAlwaysApprove))
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := app.BuildConversation(service, nil, app.NewAgents(coord), app.ConversationSpec{
		ProviderName: "protonman",
		BaseURL:      server.URL(),
		APIKey:       "mock-api-key",
		ModelID:      "mock-model",
		MaxToolCalls: 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	if conversation == nil {
		t.Fatal("conversation was not constructed")
	}

	runTurn := func(prompt, response string) {
		t.Helper()
		server.AddTextResponse(response)
		_, err := conversation.Run(context.Background(), []model.Message{{Role: model.RoleUser, Content: prompt}}, nil)
		if err != nil {
			t.Fatalf("run turn %q: %v", prompt, err)
		}
	}

	runTurn("enabled", "enabled")
	coord.SetEnabled(false)
	runTurn("disabled", "disabled")
	coord.SetEnabled(true)
	runTurn("re-enabled", "re-enabled")

	requests := server.Requests()
	if len(requests) != 3 {
		t.Fatalf("request count = %d, want 3: %#v", len(requests), requests)
	}

	if tools := requestToolNames(requests[0]); !containsString(tools, "delegate_task") {
		t.Fatalf("enabled turn missing delegate_task: %#v", tools)
	}
	if !requestMessagesContain(requests[0], "# Delegation Protocol") {
		t.Fatalf("enabled turn missing delegation prompt: %#v", requests[0]["messages"])
	}

	for _, name := range []string{"delegate_task", "wait_agent", "get_agent", "list_agents", "cancel_agent"} {
		if tools := requestToolNames(requests[1]); containsString(tools, name) {
			t.Fatalf("disabled turn published %s: %#v", name, tools)
		}
	}
	if requestMessagesContain(requests[1], "# Delegation Protocol") {
		t.Fatalf("disabled turn retained delegation prompt: %#v", requests[1]["messages"])
	}

	if tools := requestToolNames(requests[2]); !containsString(tools, "delegate_task") {
		t.Fatalf("re-enabled turn missing delegate_task: %#v", tools)
	}
	if !requestMessagesContain(requests[2], "# Delegation Protocol") {
		t.Fatalf("re-enabled turn missing delegation prompt: %#v", requests[2]["messages"])
	}
}
