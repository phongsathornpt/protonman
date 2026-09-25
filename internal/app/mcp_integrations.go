package app

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
)

// MCPIntegration is the application-owned representation of a client-managed
// stdio MCP server definition. Env contains variable names only; values are
// resolved from the client process environment when a session is created.
type MCPIntegration struct {
	Name    string
	Command string
	Args    []string
	Env     []string
}

// MCPIntegrationsRepository is the outbound persistence port for desktop-owned
// MCP definitions.
type MCPIntegrationsRepository interface {
	Load(context.Context) ([]MCPIntegration, error)
	Save(context.Context, []MCPIntegration) error
}

// MCPIntegrations owns normalization and persistence of desktop MCP settings.
type MCPIntegrations struct {
	repository MCPIntegrationsRepository
}

func NewMCPIntegrations(repository MCPIntegrationsRepository) MCPIntegrations {
	return MCPIntegrations{repository: repository}
}

func (m MCPIntegrations) Available() bool {
	return m.repository != nil
}

func (m MCPIntegrations) Load(ctx context.Context) ([]MCPIntegration, error) {
	if m.repository == nil {
		return nil, fmt.Errorf("MCP integrations repository is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	items, err := m.repository.Load(ctx)
	if err != nil {
		return nil, err
	}
	normalized, err := normalizeMCPIntegrations(items)
	if err != nil {
		return nil, err
	}
	if !equalMCPIntegrations(items, normalized) {
		if err := m.repository.Save(ctx, normalized); err != nil {
			return nil, fmt.Errorf("migrate MCP integrations: %w", err)
		}
	}
	return normalized, nil
}

func (m MCPIntegrations) Save(ctx context.Context, items []MCPIntegration) error {
	if m.repository == nil {
		return fmt.Errorf("MCP integrations repository is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	normalized, err := normalizeMCPIntegrations(items)
	if err != nil {
		return err
	}
	return m.repository.Save(ctx, normalized)
}

func normalizeMCPIntegrations(items []MCPIntegration) ([]MCPIntegration, error) {
	seen := make(map[string]struct{}, len(items))
	normalized := make([]MCPIntegration, 0, len(items))
	for _, item := range items {
		item.Name = strings.TrimSpace(item.Name)
		item.Command = strings.TrimSpace(item.Command)
		if item.Name == "" || item.Command == "" {
			return nil, fmt.Errorf("MCP integration name and command must be non-empty")
		}
		if _, exists := seen[item.Name]; exists {
			return nil, fmt.Errorf("duplicate MCP integration %q", item.Name)
		}
		seen[item.Name] = struct{}{}
		item.Args = trimMCPStringList(item.Args)
		var err error
		item.Env, err = normalizeEnvironmentKeys(item.Env)
		if err != nil {
			return nil, fmt.Errorf("MCP integration %q environment: %w", item.Name, err)
		}
		normalized = append(normalized, cloneMCPIntegration(item))
	}
	sort.Slice(normalized, func(i, j int) bool {
		return normalized[i].Name < normalized[j].Name
	})
	return normalized, nil
}

func normalizeEnvironmentKeys(values []string) ([]string, error) {
	seen := make(map[string]struct{}, len(values))
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := value
		if before, _, found := strings.Cut(value, "="); found {
			key = strings.TrimSpace(before)
		}
		if !validEnvironmentKey(key) {
			return nil, fmt.Errorf("invalid environment variable name %q", key)
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, key)
	}
	return normalized, nil
}

func validEnvironmentKey(key string) bool {
	if key == "" {
		return false
	}
	for index, character := range key {
		if index == 0 {
			if !(character == '_' || character >= 'A' && character <= 'Z' || character >= 'a' && character <= 'z') {
				return false
			}
			continue
		}
		if !(character == '_' || character >= 'A' && character <= 'Z' || character >= 'a' && character <= 'z' || character >= '0' && character <= '9') {
			return false
		}
	}
	return true
}

func trimMCPStringList(values []string) []string {
	trimmed := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			trimmed = append(trimmed, value)
		}
	}
	return trimmed
}

func cloneMCPIntegration(item MCPIntegration) MCPIntegration {
	item.Args = append([]string(nil), item.Args...)
	item.Env = append([]string(nil), item.Env...)
	return item
}

func equalMCPIntegrations(left, right []MCPIntegration) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].Name != right[index].Name || left[index].Command != right[index].Command ||
			!slices.Equal(left[index].Args, right[index].Args) || !slices.Equal(left[index].Env, right[index].Env) {
			return false
		}
	}
	return true
}
