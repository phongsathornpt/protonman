package app

import (
	"context"

	"github.com/phongsathornpt/proton/internal/adapter/out/config"
	"github.com/phongsathornpt/proton/internal/core/permission"
	"github.com/phongsathornpt/proton/internal/feature/project"
	sdk "github.com/phongsathornpt/proton/proton-sdk"
)

// ProjectDiscoveryOptions controls project-local Proton discovery.
type ProjectDiscoveryOptions = project.Options

// ProjectState describes project-local Proton resources.
type ProjectState = project.State

// ProjectInitResult reports whether project-local Proton configuration was created.
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
