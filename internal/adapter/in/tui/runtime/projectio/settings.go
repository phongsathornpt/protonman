package projectio

import (
	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

func SaveAgentProfile(workDir, profile string) error {
	return (app.Projects{}).SaveAgentProfile(workDir, profile)
}
func SaveSubagentsEnabled(workDir string, enabled bool) error {
	return (app.Projects{}).SaveSubagentsEnabled(workDir, enabled)
}
func SaveReasoningEffort(workDir string, effort sdk.ReasoningEffort) error {
	return (app.Projects{}).SaveReasoningEffort(workDir, effort)
}
func SaveMaxToolCalls(workDir string, calls int) error {
	return (app.Projects{}).SaveMaxToolCalls(workDir, calls)
}
func SavePermissionMode(workDir string, mode permission.Mode) error {
	return (app.Projects{}).SavePermissionMode(workDir, mode)
}
