package app

import (
	"github.com/projectTHORN/proton/internal/config"
	"github.com/projectTHORN/proton/internal/permission"
	sdk "github.com/projectTHORN/proton/proton-sdk"
)

// Projects owns project-local configuration mutations used by inbound adapters.
type Projects struct{}

func (Projects) SaveAgentProfile(workDir, profile string) error {
	return config.SaveProjectAgentProfile(workDir, profile)
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
