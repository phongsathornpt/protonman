package main

import (
	"context"
	"fmt"
	"os"

	"github.com/phongsathornpt/proton/internal/adapter/in/headless"
	"github.com/phongsathornpt/proton/internal/app"
	"github.com/phongsathornpt/proton/internal/core/session"
	"github.com/phongsathornpt/proton/internal/core/tool"
	"github.com/phongsathornpt/proton/internal/engine/toolcall"
	"github.com/phongsathornpt/proton/internal/feature/skill"
)

func runHeadless(
	ctx context.Context,
	service *toolcall.Service,
	registry tool.Registry,
	skillRegistry *skill.Registry,
	stateStore session.Repository,
	sessionID string,
	state session.State,
	prompt string,
	outputFormat string,
	turnRunner app.Conversation,
) error {
	format, err := headless.ParseFormat(outputFormat)
	if err != nil {
		return err
	}
	runner, err := headless.New(service, registry, turnRunner, headless.WithSkills(skillRegistry))
	if err != nil {
		return fmt.Errorf("create headless runner: %w", err)
	}
	if err := runner.LoadSession(state); err != nil {
		return fmt.Errorf("restore session transcript: %w", err)
	}
	runErr := runner.Run(ctx, prompt, os.Stdout, format)
	var activeSkills []string
	if skillRegistry != nil {
		activeSkills = skillRegistry.ActivatedList()
	}
	saveErr := stateStore.Save(ctx, sessionID, session.State{
		SessionID:       sessionID,
		Revision:        state.Revision,
		WorkspaceKey:    state.WorkspaceKey,
		WorkspaceName:   state.WorkspaceName,
		CreatedAt:       state.CreatedAt,
		PermissionMode:  service.Mode().String(),
		ActiveSkills:    activeSkills,
		AgentProfile:    state.AgentProfile,
		ReasoningEffort: state.ReasoningEffort,
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
