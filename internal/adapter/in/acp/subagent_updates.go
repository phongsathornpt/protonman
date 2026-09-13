package acp

import (
	"context"
	"strings"

	"github.com/phongsathornpt/protonman/internal/feature/agent"
)

const sessionUpdateSubagent = "protonman_subagent_update"

func (s *Session) startSubagentNotifications(ctx context.Context, notifier func(RPCNotification) error) func() {
	if notifier == nil || !s.agents.Available() {
		return func() {}
	}
	events, unsubscribe := s.agents.Subscribe(32)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-events:
				if !ok {
					return
				}
				if ev.SessionID != "" && ev.SessionID != s.id {
					continue
				}
				_ = s.notifySubagentEvent(notifier, ev)
			}
		}
	}()
	return func() {
		unsubscribe()
		<-done
	}
}

func (s *Session) notifySubagentEvent(notifier func(RPCNotification) error, ev agent.Event) error {
	status, found := s.subagentStatus(ev.AgentID)
	profile := string(ev.Profile)
	task := ""
	state := subagentStateForEvent(ev.Kind)
	summary := strings.TrimSpace(ev.Message)
	if found {
		profile = string(status.Profile)
		task = status.Task
		state = string(status.State)
		if summary == "" {
			summary = status.Reason
		}
	}
	return notifySubagent(notifier, s.id, ev.AgentID, profile, task, state, summary)
}

func (s *Session) subagentStatus(agentID string) (agent.AgentStatus, bool) {
	for _, status := range s.agents.List() {
		if status.ID == agentID {
			return status, true
		}
	}
	return agent.AgentStatus{}, false
}

func (s *Session) replaySubagents(notifier func(RPCNotification) error) error {
	if notifier == nil || !s.agents.Available() {
		return nil
	}
	for _, status := range s.agents.List() {
		if err := notifySubagent(notifier, s.id, status.ID, string(status.Profile), status.Task, string(status.State), status.Reason); err != nil {
			return err
		}
	}
	return nil
}

func notifySubagent(notifier func(RPCNotification) error, sessionID, agentID, profile, task, status, summary string) error {
	return notifier(RPCNotification{JSONRPC: "2.0", Method: "session/update", Params: map[string]any{
		"sessionId": sessionID,
		"update": map[string]any{
			"sessionUpdate": sessionUpdateSubagent,
			"agentId":       agentID,
			"profile":       profile,
			"task":          task,
			"status":        status,
			"summary":       summary,
		},
	}})
}

func subagentStateForEvent(kind agent.EventKind) string {
	switch kind {
	case agent.EventAgentQueued:
		return string(agent.StateQueued)
	case agent.EventAgentStarted, agent.EventAgentProgress:
		return string(agent.StateRunning)
	case agent.EventAgentCompleted:
		return string(agent.StateCompleted)
	case agent.EventAgentFailed:
		return string(agent.StateFailed)
	default:
		return string(kind)
	}
}
