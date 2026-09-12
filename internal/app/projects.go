package app

import (
	"context"
	"fmt"

	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/feature/project"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

type ProjectDiscoveryOptions = project.Options
type ProjectState = project.State
type ProjectInitResult = project.InitResult

// ProjectSettingsRepository persists trusted project-local preferences.
type ProjectSettingsRepository interface {
	SaveAgentProfile(string, string) error
	SaveSubagentsEnabled(string, bool) error
	SaveReasoningEffort(string, sdk.ReasoningEffort) error
	SaveMaxToolCalls(string, int) error
	SavePermissionMode(string, permission.Mode) error
	SavePermissionRule(string, permission.Rule) error
}

// Projects owns project-local configuration mutations and lifecycle use cases.
type Projects struct{ repository ProjectSettingsRepository }

func NewProjects(repository ProjectSettingsRepository) Projects {
	return Projects{repository: repository}
}

func (Projects) Discover(ctx context.Context, opts ProjectDiscoveryOptions) (ProjectState, error) {
	return project.Discover(ctx, opts)
}

func (Projects) Init(ctx context.Context, workDir string) (ProjectInitResult, error) {
	return project.Init(ctx, workDir)
}

func (p Projects) SaveAgentProfile(workDir, profile string) error {
	if p.repository == nil {
		return fmt.Errorf("project settings repository is unavailable")
	}
	return p.repository.SaveAgentProfile(workDir, profile)
}

func (p Projects) SaveSubagentsEnabled(workDir string, enabled bool) error {
	if p.repository == nil {
		return fmt.Errorf("project settings repository is unavailable")
	}
	return p.repository.SaveSubagentsEnabled(workDir, enabled)
}

func (p Projects) SaveReasoningEffort(workDir string, effort sdk.ReasoningEffort) error {
	if p.repository == nil {
		return fmt.Errorf("project settings repository is unavailable")
	}
	return p.repository.SaveReasoningEffort(workDir, effort)
}

func (p Projects) SaveMaxToolCalls(workDir string, calls int) error {
	if p.repository == nil {
		return fmt.Errorf("project settings repository is unavailable")
	}
	return p.repository.SaveMaxToolCalls(workDir, calls)
}

func (p Projects) SavePermissionMode(workDir string, mode permission.Mode) error {
	if p.repository == nil {
		return fmt.Errorf("project settings repository is unavailable")
	}
	return p.repository.SavePermissionMode(workDir, mode)
}

func (p Projects) SavePermissionRule(workDir string, rule permission.Rule) error {
	if p.repository == nil {
		return fmt.Errorf("project settings repository is unavailable")
	}
	return p.repository.SavePermissionRule(workDir, rule)
}
