package app

import (
	"context"
	"fmt"

	"github.com/phongsathornpt/protonman/internal/core/permission"
	project "github.com/phongsathornpt/protonman/internal/core/project"
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
	SaveActiveSkills(string, []string) error
}

// ProjectLifecycle is the application-owned port for project discovery and
// initialization. The filesystem implementation lives in internal/feature/project
// and is wired at the composition root.
type ProjectLifecycle interface {
	Discover(ctx context.Context, opts ProjectDiscoveryOptions) (ProjectState, error)
	Init(ctx context.Context, workDir string) (ProjectInitResult, error)
}

// Projects owns project-local configuration mutations and lifecycle use cases.
type Projects struct {
	repository ProjectSettingsRepository
	lifecycle  ProjectLifecycle
}

// NewProjects builds the project use case. The lifecycle port must be supplied
// by the composition root; use cases fail closed without it.
func NewProjects(repository ProjectSettingsRepository, lifecycle ProjectLifecycle) Projects {
	return Projects{repository: repository, lifecycle: lifecycle}
}

// Discover inspects project-local Protonman resources.
func (p Projects) Discover(ctx context.Context, opts ProjectDiscoveryOptions) (ProjectState, error) {
	if p.lifecycle == nil {
		return ProjectState{}, fmt.Errorf("project lifecycle is unavailable")
	}
	return p.lifecycle.Discover(ctx, opts)
}

// Init creates a minimal project-local Protonman configuration.
func (p Projects) Init(ctx context.Context, workDir string) (ProjectInitResult, error) {
	if p.lifecycle == nil {
		return ProjectInitResult{}, fmt.Errorf("project lifecycle is unavailable")
	}
	return p.lifecycle.Init(ctx, workDir)
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

func (p Projects) SaveActiveSkills(workDir string, activeSkills []string) error {
	if p.repository == nil {
		return fmt.Errorf("project settings repository is unavailable")
	}
	return p.repository.SaveActiveSkills(workDir, activeSkills)
}
