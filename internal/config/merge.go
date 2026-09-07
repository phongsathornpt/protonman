package config

import (
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/projectTHORN/proton/internal/permission"
	"github.com/projectTHORN/proton/internal/sandbox"
	sdk "github.com/projectTHORN/proton/proton-sdk"
)

func mergeDocument(document fileDocument, snapshot *Snapshot, source ValueSource) error {
	if document.Permission.Default != "" {
		defaultAction, err := permission.ParseAction(document.Permission.Default)
		if err != nil {
			return fmt.Errorf("permission.default: %w", err)
		}
		snapshot.Permission.Default = defaultAction
	}
	for index, rawRule := range document.Permission.Rules {
		rule, err := decodeRule(rawRule)
		if err != nil {
			return fmt.Errorf("permission.rules[%d]: %w", index, err)
		}
		snapshot.Permission.Rules = append(snapshot.Permission.Rules, rule)
	}
	for _, protectedPath := range document.Workspace.ProtectedPaths {
		protectedPath = strings.TrimSpace(protectedPath)
		if protectedPath == "" {
			return fmt.Errorf("workspace.protected_paths contains an empty path")
		}
		snapshot.ProtectedPaths = append(snapshot.ProtectedPaths, protectedPath)
	}
	if document.UI.PermissionMode != "" {
		mode, err := permission.ParseMode(document.UI.PermissionMode)
		if err != nil {
			return fmt.Errorf("ui.permission_mode: %w", err)
		}
		snapshot.Mode = mode
		snapshot.Provenance[FieldUIPermissionMode] = source
	}
	if document.Sandbox.Profile != "" {
		name, err := sandbox.ParseName(document.Sandbox.Profile)
		if err != nil {
			return fmt.Errorf("sandbox.profile: %w", err)
		}
		snapshot.Sandbox = name
	}
	if len(document.Providers) > 0 {
		if snapshot.Providers == nil {
			snapshot.Providers = make(map[string]ProviderConfig)
		}
		maps.Copy(snapshot.Providers, document.Providers)
	}
	if document.Model.Default != "" {
		snapshot.Model.Default = document.Model.Default
		snapshot.Provenance[FieldModelDefault] = source
	}
	if document.Model.Provider != "" {
		snapshot.Model.Provider = document.Model.Provider
		snapshot.Provenance[FieldModelProvider] = source
	}
	if document.Agent.MaxToolCalls != nil {
		if *document.Agent.MaxToolCalls < 0 {
			return fmt.Errorf("agent.max_tool_calls must be non-negative")
		}
		snapshot.Agent.MaxToolCalls = *document.Agent.MaxToolCalls
		snapshot.Provenance[FieldAgentMaxToolCalls] = source
	}
	if document.Agent.Profile != nil {
		snapshot.Agent.Profile = strings.TrimSpace(*document.Agent.Profile)
		snapshot.Provenance[FieldAgentProfile] = source
	}
	if document.Agent.ReasoningEffort != nil {
		effort, err := sdk.ParseReasoningEffort(*document.Agent.ReasoningEffort)
		if err != nil {
			return fmt.Errorf("agent.reasoning_effort: %w", err)
		}
		snapshot.Agent.ReasoningEffort = effort
		snapshot.Provenance[FieldAgentReasoningEffort] = source
	}
	if document.Agent.MaxLiveSubagents != nil {
		if *document.Agent.MaxLiveSubagents <= 0 {
			return fmt.Errorf("agent.max_live_subagents must be positive")
		}
		snapshot.Agent.MaxLiveSubagents = *document.Agent.MaxLiveSubagents
	}
	if document.Agent.MaxRetainedSubagents != nil {
		if *document.Agent.MaxRetainedSubagents <= 0 {
			return fmt.Errorf("agent.max_retained_subagents must be positive")
		}
		snapshot.Agent.MaxRetainedSubagents = *document.Agent.MaxRetainedSubagents
	}
	if document.Agent.SubagentMaxRuntime != nil {
		d, err := parsePositiveDuration("agent.subagent_max_runtime", *document.Agent.SubagentMaxRuntime)
		if err != nil {
			return err
		}
		snapshot.Agent.SubagentMaxRuntime = d
	} else if document.Agent.SubagentTimeout != nil {
		d, err := parsePositiveDuration("agent.subagent_timeout", *document.Agent.SubagentTimeout)
		if err != nil {
			return err
		}
		snapshot.Agent.SubagentMaxRuntime = d
		snapshot.Warnings = append(snapshot.Warnings, "agent.subagent_timeout is deprecated; use agent.subagent_max_runtime")
	}
	if document.Agent.SubagentWaitTimeout != nil {
		d, err := parsePositiveDuration("agent.subagent_wait_timeout", *document.Agent.SubagentWaitTimeout)
		if err != nil {
			return err
		}
		snapshot.Agent.SubagentWaitTimeout = d
	}
	if document.Agent.SubagentQueueTimeout != nil {
		d, err := parsePositiveDuration("agent.subagent_queue_timeout", *document.Agent.SubagentQueueTimeout)
		if err != nil {
			return err
		}
		snapshot.Agent.SubagentQueueTimeout = d
	}
	if document.Agent.CompletedResultTTL != nil {
		d, err := parsePositiveDuration("agent.completed_result_ttl", *document.Agent.CompletedResultTTL)
		if err != nil {
			return err
		}
		snapshot.Agent.CompletedResultTTL = d
	}

	for field, target := range map[string]struct {
		raw *string
		set func(time.Duration)
	}{
		"runtime.turn_timeout":            {document.Runtime.TurnTimeout, func(d time.Duration) { snapshot.Runtime.TurnTimeout = d }},
		"runtime.round_timeout":           {document.Runtime.RoundTimeout, func(d time.Duration) { snapshot.Runtime.RoundTimeout = d }},
		"runtime.tool_permission_timeout": {document.Runtime.ToolPermissionTimeout, func(d time.Duration) { snapshot.Runtime.ToolPermissionTimeout = d }},
		"runtime.tool_execution_timeout":  {document.Runtime.ToolExecutionTimeout, func(d time.Duration) { snapshot.Runtime.ToolExecutionTimeout = d }},
		"runtime.model_request_timeout":   {document.Runtime.ModelRequestTimeout, func(d time.Duration) { snapshot.Runtime.ModelRequestTimeout = d }},
		"runtime.model_discovery_timeout": {document.Runtime.ModelDiscoveryTimeout, func(d time.Duration) { snapshot.Runtime.ModelDiscoveryTimeout = d }},
		"runtime.web_fetch_timeout":       {document.Runtime.WebFetchTimeout, func(d time.Duration) { snapshot.Runtime.WebFetchTimeout = d }},
		"runtime.model_catalog_ttl":       {document.Runtime.ModelCatalogTTL, func(d time.Duration) { snapshot.Runtime.ModelCatalogTTL = d }},
	} {
		if target.raw == nil {
			continue
		}
		d, err := parsePositiveDuration(field, *target.raw)
		if err != nil {
			return err
		}
		target.set(d)
	}
	return nil
}

func parsePositiveDuration(field, raw string) (time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, fmt.Errorf("%s must not be empty", field)
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", field, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("%s must be greater than zero", field)
	}
	return d, nil
}

// SaveUserProviderConfig persists or updates a provider configuration in ~/.proton/config.toml.
func decodeRule(raw fileRule) (permission.Rule, error) {
	// Grok defaults omitted rule actions to deny. Keeping that default avoids
	// turning a partially written rule into an accidental allow.
	action := permission.ActionDeny
	if strings.TrimSpace(raw.Action) != "" {
		parsedAction, err := permission.ParseAction(raw.Action)
		if err != nil {
			return permission.Rule{}, fmt.Errorf("action: %w", err)
		}
		action = parsedAction
	}
	toolKind, err := permission.ParseToolKind(raw.Tool)
	if err != nil {
		return permission.Rule{}, fmt.Errorf("tool: %w", err)
	}
	patternMode, err := permission.ParsePatternMode(raw.PatternMode)
	if err != nil {
		return permission.Rule{}, fmt.Errorf("pattern_mode: %w", err)
	}
	return permission.Rule{
		Action:      action,
		Tool:        toolKind,
		Pattern:     raw.Pattern,
		PatternMode: patternMode,
	}, nil
}
