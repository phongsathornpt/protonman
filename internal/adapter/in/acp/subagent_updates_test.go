package acp

import (
	"encoding/json"
	"testing"

	"github.com/phongsathornpt/protonman/internal/feature/agent"
)

func TestNotifySubagentBuildsStructuredSessionUpdate(t *testing.T) {
	var got RPCNotification
	err := notifySubagent(func(notification RPCNotification) error {
		got = notification
		return nil
	}, "s1", "a1", "strength", "Implement ACP", "running", "editing session adapter")
	if err != nil {
		t.Fatal(err)
	}
	if got.Method != "session/update" {
		t.Fatalf("method = %q", got.Method)
	}
	encoded, err := json.Marshal(got.Params)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		SessionID string `json:"sessionId"`
		Update struct {
			Kind    string `json:"sessionUpdate"`
			AgentID string `json:"agentId"`
			Profile string `json:"profile"`
			Task    string `json:"task"`
			Status  string `json:"status"`
			Summary string `json:"summary"`
		} `json:"update"`
	}
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.SessionID != "s1" || payload.Update.Kind != sessionUpdateSubagent {
		t.Fatalf("payload = %#v", payload)
	}
	if payload.Update.AgentID != "a1" || payload.Update.Profile != "strength" || payload.Update.Status != "running" {
		t.Fatalf("update = %#v", payload.Update)
	}
	if payload.Update.Task != "Implement ACP" || payload.Update.Summary != "editing session adapter" {
		t.Fatalf("content = %#v", payload.Update)
	}
}

func TestSubagentStateForEvent(t *testing.T) {
	cases := map[agent.EventKind]string{
		agent.EventAgentQueued:    "queued",
		agent.EventAgentStarted:   "running",
		agent.EventAgentProgress:  "running",
		agent.EventAgentCompleted: "completed",
		agent.EventAgentFailed:    "failed",
	}
	for kind, want := range cases {
		if got := subagentStateForEvent(kind); got != want {
			t.Fatalf("%s => %q, want %q", kind, got, want)
		}
	}
}
