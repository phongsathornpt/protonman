package main

import (
	"context"
	"fmt"
	"os"

	"github.com/projectTHORN/proton/internal/headless"
	"github.com/projectTHORN/proton/internal/session"
	"github.com/projectTHORN/proton/internal/skill"
	"github.com/projectTHORN/proton/internal/tool"
	"github.com/projectTHORN/proton/internal/toolcall"
	"github.com/projectTHORN/proton/internal/turn"
)

func runHeadless(
	ctx context.Context,
	service *toolcall.Service,
	registry tool.Registry,
	skillRegistry *skill.Registry,
	stateStore *session.FileStore,
	sessionID string,
	state session.State,
	prompt string,
	outputFormat string,
	turnRunner turn.Runner,
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
