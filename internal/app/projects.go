package app

import (
	"context"

	"github.com/phongsathornpt/protonman/internal/adapter/out/config"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	"github.com/phongsathornpt/protonman/internal/feature/project"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

// ProjectDiscoveryOptions controls project-local Protonman discovery.
type ProjectDiscoveryOptions = project.Options

// ProjectState describes project-local Protonman resources.
type ProjectState = project.State

// ProjectInitResult reports whether project-local Protonman configuration was created.
type ProjectInitResult = project.InitResult

// Projects owns project-local configuration mutations and lifecycle use cases.
type Projects struct{}

func (Projects) Discover(ctx context.Context, opts ProjectDiscoveryOptions) (ProjectState, error) {
	return project.Discover(ctx, opts)
}

func (Projects) Init(ctx context.Context, workDir string) (ProjectInitResult, error) {
	return project.Init(ctx, workDir)
}

func (Projects) SaveAgentProfile(workDir, profile string) error {
	return config.SaveProjectAgentProfile(workDir, profile)
}

func (Projects) SaveSubagentsEnabled(workDir string, enabled bool) error {
	return config.SaveProjectSubagentsEnabled(workDir, enabled)
}

func (Projects) SaveReasoningEffort(workDir string, effort sdk.ReasoningEffort) error {
	return config.SaveProjectReasoningEffort(workDir, effort)
}

func (Projects) SaveMaxToolCalls(workDir string, calls int) error {
	return config.SaveProjectMaxToolCalls(workDir, calls)
}

func (Projects) SavePermissionMode(workDir string, mode permission.Mode) error {
	return config.SaveProjectPermissionMode(workDir, mode)
}

func (Projects) SavePermissionRule(workDir string, rule permission.Rule) error {
	return config.SaveProjectPermissionRule(workDir, rule)
}
