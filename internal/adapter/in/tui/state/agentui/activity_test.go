package agentui

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/feature/agent"
)

func TestActivityForStateUsesDotaPresentationVocabulary(t *testing.T) {
	cases := []struct {
		profile agent.Profile
		state   agent.State
		want    string
	}{
		{agent.ProfileAgility, agent.StateQueued, "W8"},
		{agent.ProfileAgility, agent.StateRunning, "Roaming"},
		{agent.ProfileIntelligence, agent.StateRunning, "Skilling"},
		{agent.ProfileStrength, agent.StateRunning, "Pushing"},
		{agent.ProfileStrength, agent.StateCanceling, "B"},
		{agent.ProfileAgility, agent.StateFailed, "Care"},
		{agent.ProfileAgility, agent.StateCompleted, "Ready"},
	}
	for _, tc := range cases {
		if got := ActivityForState(tc.profile, tc.state).String(); got != tc.want {
			t.Fatalf("activity(%s,%s)=%q, want %q", tc.profile, tc.state, got, tc.want)
		}
	}
}

func TestActivityFromEventProjectsToolSemantics(t *testing.T) {
	call := func(name, args string) *tool.Call {
		c, err := tool.NewCall("call-1", name, json.RawMessage(args))
		if err != nil {
			t.Fatal(err)
		}
		return &c
	}
	cases := []struct {
		event agent.Event
		want  ActivityIntent
	}{
		{agent.Event{Kind: agent.EventAgentProgress, Profile: agent.ProfileAgility, Call: call("read", `{"path":"session.go"}`)}, ActivityFarming},
		{agent.Event{Kind: agent.EventAgentProgress, Profile: agent.ProfileAgility, Call: call("grep", `{"pattern":"race"}`)}, ActivityGanking},
		{agent.Event{Kind: agent.EventAgentProgress, Profile: agent.ProfileStrength, Call: call("edit", `{"action":"write","file_path":"session.go","content":"x"}`)}, ActivityPushing},
		{agent.Event{Kind: agent.EventAgentProgress, Profile: agent.ProfileStrength, Call: call("bash", `{"command":"go test -race ./..."}`)}, ActivityDefending},
		{agent.Event{Kind: agent.EventAgentResultAvailable, Profile: agent.ProfileAgility}, ActivitySticking},
		{agent.Event{Kind: agent.EventAgentFailed, Profile: agent.ProfileAgility, Err: context.Canceled}, ActivityRetreating},
	}
	for _, tc := range cases {
		if got := ActivityFromEvent(tc.event).Intent; got != tc.want {
			t.Fatalf("event %s activity=%q, want %q", tc.event.Kind, got, tc.want)
		}
	}
}
