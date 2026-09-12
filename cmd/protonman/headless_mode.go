package main

import (
	"context"
	"fmt"
	"os"

	"github.com/phongsathornpt/protonman/internal/adapter/in/headless"
	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/core/session"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/engine/toolcall"
	"github.com/phongsathornpt/protonman/internal/feature/skill"
)

type headlessInvocation struct {
	service       *toolcall.Service
	registry      tool.Registry
	skillRegistry *skill.Registry
	stateStore    session.Repository
	sessionID     string
	state         session.State
	prompt        string
	outputFormat  string
	turnRunner    app.Conversation
	agents        app.Agents
}

func runHeadless(ctx context.Context, inv headlessInvocation) error {
	format, err := headless.ParseFormat(inv.outputFormat)
	if err != nil {
		return err
	}
	runner, err := headless.New(
		inv.service,
		inv.registry,
		inv.turnRunner,
		headless.WithSkills(inv.skillRegistry),
		headless.WithSessionID(inv.sessionID),
		headless.WithAgents(inv.agents.ForSession(inv.sessionID)),
	)
	if err != nil {
		return fmt.Errorf("create headless runner: %w", err)
	}
	if err := runner.LoadSession(inv.state); err != nil {
		return fmt.Errorf("restore session transcript: %w", err)
	}
	runErr := runner.Run(ctx, inv.prompt, os.Stdout, format)
	activeSkills := []string{}
	if inv.skillRegistry != nil {
		activeSkills = inv.skillRegistry.ActivatedList()
	}
	saveErr := inv.stateStore.Save(ctx, inv.sessionID, session.State{
		SessionID:       inv.sessionID,
		Revision:        inv.state.Revision,
		WorkspaceKey:    inv.state.WorkspaceKey,
		WorkspaceName:   inv.state.WorkspaceName,
		CreatedAt:       inv.state.CreatedAt,
		PermissionMode:  inv.service.Mode().String(),
		ActiveSkills:    activeSkills,
		ActiveGoal:      inv.state.ActiveGoal,
		AgentProfile:    inv.state.AgentProfile,
		ReasoningEffort: inv.state.ReasoningEffort,
		Messages:        runner.SessionState(),
	})
	if runErr != nil && saveErr != nil {
		return fmt.Errorf("run headless: %v; save session: %w", runErr, saveErr)
	}
	if runErr != nil {
		return fmt.Errorf("run headless: %w", runErr)
	}
	if saveErr != nil {
		return fmt.Errorf("save session: %w", saveErr)
	}
	return nil
}
