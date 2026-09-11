package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/phongsathornpt/protonman/internal/app/appdirs"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

var projectConfigMutationMu sync.Mutex

// ErrProjectScopeUnavailable indicates that project-local state aliases user-global state.
var ErrProjectScopeUnavailable = errors.New("project scope is unavailable")

// SaveProjectAgentProfile updates the project-local agent profile.
func SaveProjectAgentProfile(workDir, profile string) error {
	profile = strings.TrimSpace(profile)
	if profile == "" {
		return fmt.Errorf("agent profile must not be empty")
	}
	return modifyProjectConfigFile(workDir, func(doc *fileDocument) {
		doc.Agent.Profile = &profile
	})
}

// SaveProjectSubagentsEnabled updates the project-local subagent capability switch.
func SaveProjectSubagentsEnabled(workDir string, enabled bool) error {
	return modifyProjectConfigFile(workDir, func(doc *fileDocument) {
		doc.Agent.SubagentsEnabled = &enabled
	})
}

// SaveProjectReasoningEffort updates the project-local reasoning preference.
func SaveProjectReasoningEffort(workDir string, effort sdk.ReasoningEffort) error {
	if !effort.Valid() {
		return fmt.Errorf("invalid reasoning effort %q", effort)
	}
	value := string(effort)
	if effort == sdk.ReasoningDefault {
		value = "auto"
	}
	return modifyProjectConfigFile(workDir, func(doc *fileDocument) {
		doc.Agent.ReasoningEffort = &value
	})
}

// SaveProjectMaxToolCalls updates the project-local cumulative tool-call limit.
func SaveProjectMaxToolCalls(workDir string, maxToolCalls int) error {
	if maxToolCalls < 0 {
		return fmt.Errorf("max tool calls cannot be negative")
	}
	return modifyProjectConfigFile(workDir, func(doc *fileDocument) {
		doc.Agent.MaxToolCalls = &maxToolCalls
	})
}

// SaveProjectPermissionMode updates the project-local initial permission mode.
func SaveProjectPermissionMode(workDir string, mode permission.Mode) error {
	if _, err := permission.ParseMode(mode.String()); err != nil {
		return err
	}
	value := mode.String()
	return modifyProjectConfigFile(workDir, func(doc *fileDocument) {
		doc.UI.PermissionMode = value
	})
}

// SaveProjectPermissionRule adds a static permission rule to the project config.
func SaveProjectPermissionRule(workDir string, rule permission.Rule) error {
	if !rule.Action.Valid() {
		return fmt.Errorf("invalid permission action: %v", rule.Action)
	}
	if !permission.ValidToolKind(rule.Tool) {
		return fmt.Errorf("invalid permission tool kind: %q", rule.Tool)
	}
	return modifyProjectConfigFile(workDir, func(doc *fileDocument) {
		appendRuleToDocument(doc, rule)
	})
}

func appendRuleToDocument(doc *fileDocument, rule permission.Rule) {
	patternModeStr := ""
	if rule.PatternMode == permission.PatternModeDomain {
		patternModeStr = "domain"
	}
	pattern := rule.Pattern
	if rule.PatternMode != permission.PatternModeDomain && (strings.EqualFold(strings.TrimSpace(pattern), "all") || strings.TrimSpace(pattern) == "") {
		pattern = "*"
	}
	raw := fileRule{
		Action:      rule.Action.String(),
		Tool:        string(rule.Tool),
		Pattern:     pattern,
		PatternMode: patternModeStr,
	}
	for _, existing := range doc.Permission.Rules {
		existingMode := existing.PatternMode
		if existingMode == "glob" {
			existingMode = ""
		}
		existingPattern := existing.Pattern
		if existingMode != "domain" && (strings.EqualFold(strings.TrimSpace(existingPattern), "all") || strings.TrimSpace(existingPattern) == "") {
			existingPattern = "*"
		}
		if existing.Action == raw.Action &&
			strings.EqualFold(existing.Tool, raw.Tool) &&
			existingPattern == raw.Pattern &&
			existingMode == raw.PatternMode {
			return
		}
	}
	doc.Permission.Rules = append(doc.Permission.Rules, raw)
}

func modifyProjectConfigFile(workDir string, mutate func(*fileDocument)) error {
	projectConfigMutationMu.Lock()
	defer projectConfigMutationMu.Unlock()
	absWorkDir, err := filepath.Abs(strings.TrimSpace(workDir))
	if err != nil {
		return fmt.Errorf("resolve project work directory: %w", err)
	}
	dirs, err := appdirs.Resolve("")
	if err != nil {
		return fmt.Errorf("resolve user protonman state: %w", err)
	}
	scope, err := appdirs.ResolveProjectScope(dirs.Home, absWorkDir)
	if err != nil {
		return fmt.Errorf("resolve project scope: %w", err)
	}
	if !scope.Available {
		return fmt.Errorf("%w: project settings cannot target user-global Protonman state", ErrProjectScopeUnavailable)
	}
	root := scope.Root
	if info, statErr := os.Lstat(root); statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing project config write through symlink: %s", root)
		}
		if !info.IsDir() {
			return fmt.Errorf("project protonman path is not a directory: %s", root)
		}
	} else if errors.Is(statErr, os.ErrNotExist) {
		if err := os.Mkdir(root, 0o755); err != nil && !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("create project protonman directory: %w", err)
		}
	} else {
		return fmt.Errorf("inspect project protonman directory: %w", statErr)
	}

	path := scope.Config
	doc, _, err := readDocument(path, "project config", true)
	if err != nil {
		return err
	}
	mutate(&doc)
	return writeDocumentAtomic(root, path, "project config", 0o644, doc)
}
