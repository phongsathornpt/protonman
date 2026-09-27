package app

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
)

// ACPAgentProfile is the application-owned representation of one desktop ACP
// process. Persisted profiles contain environment variable names only. A
// process-level environment override may retain KEY=value entries for that run;
// those values are never written to the repository.
type ACPAgentProfile struct {
	ID          string
	DisplayName string
	Command     string
	Args        []string
	Env         []string
}

// ACPAgentsRepository is the outbound persistence port for desktop-owned ACP
// agent profiles.
type ACPAgentsRepository interface {
	Load(context.Context) ([]ACPAgentProfile, error)
	Save(context.Context, []ACPAgentProfile) error
}

// ACPAgents owns validation, environment-key scrubbing, and persistence for
// desktop ACP profiles.
type ACPAgents struct {
	repository ACPAgentsRepository
}

func NewACPAgents(repository ACPAgentsRepository) ACPAgents {
	return ACPAgents{repository: repository}
}

func (a ACPAgents) Available() bool {
	return a.repository != nil
}

// Resolve returns the environment override when present, otherwise the saved
// profiles, otherwise the supplied built-in defaults.
func (a ACPAgents) Resolve(ctx context.Context, defaults []ACPAgentProfile, overrideJSON string) ([]ACPAgentProfile, error) {
	if strings.TrimSpace(overrideJSON) != "" {
		return ParseACPAgentsJSONOverride(overrideJSON)
	}
	if !a.Available() {
		return normalizeACPAgents(defaults)
	}
	profiles, err := a.Load(ctx)
	if err != nil {
		return nil, err
	}
	if len(profiles) == 0 {
		return normalizeACPAgents(defaults)
	}
	return profiles, nil
}

func (a ACPAgents) Load(ctx context.Context) ([]ACPAgentProfile, error) {
	if a.repository == nil {
		return nil, fmt.Errorf("ACP agents repository is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	profiles, err := a.repository.Load(ctx)
	if err != nil {
		return nil, err
	}
	if len(profiles) == 0 {
		return nil, nil
	}
	normalized, err := normalizeACPAgents(profiles)
	if err != nil {
		return nil, err
	}
	if !equalACPAgents(profiles, normalized) {
		if err := a.repository.Save(ctx, normalized); err != nil {
			return nil, fmt.Errorf("migrate ACP agent profiles: %w", err)
		}
	}
	return normalized, nil
}

func (a ACPAgents) Save(ctx context.Context, profiles []ACPAgentProfile) error {
	if a.repository == nil {
		return fmt.Errorf("ACP agents repository is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	normalized, err := normalizeACPAgents(profiles)
	if err != nil {
		return err
	}
	return a.repository.Save(ctx, normalized)
}

// ParseACPAgentsJSON parses the PROTONMAN_ACP_AGENTS_JSON representation.
func ParseACPAgentsJSON(raw string) ([]ACPAgentProfile, error) {
	return parseACPAgentsJSON(raw, true)
}

// ParseACPAgentsJSONOverride parses the process override while retaining
// KEY=value environment entries for runtime process construction.
func ParseACPAgentsJSONOverride(raw string) ([]ACPAgentProfile, error) {
	return parseACPAgentsJSON(raw, false)
}

func parseACPAgentsJSON(raw string, scrubEnvironment bool) ([]ACPAgentProfile, error) {
	var configured []ACPAgentProfile
	if err := json.Unmarshal([]byte(raw), &configured); err != nil {
		return nil, fmt.Errorf("decode ACP agents JSON: %w", err)
	}
	return normalizeACPAgentsWithEnvironment(configured, scrubEnvironment)
}

func normalizeACPAgents(profiles []ACPAgentProfile) ([]ACPAgentProfile, error) {
	return normalizeACPAgentsWithEnvironment(profiles, true)
}

func normalizeACPAgentsWithEnvironment(profiles []ACPAgentProfile, scrubEnvironment bool) ([]ACPAgentProfile, error) {
	if len(profiles) == 0 {
		return nil, fmt.Errorf("at least one ACP agent profile is required")
	}
	seen := make(map[string]struct{}, len(profiles))
	normalized := make([]ACPAgentProfile, 0, len(profiles))
	for _, profile := range profiles {
		profile.ID = strings.ToLower(strings.TrimSpace(profile.ID))
		profile.DisplayName = strings.TrimSpace(profile.DisplayName)
		profile.Command = strings.TrimSpace(profile.Command)
		if !validACPAgentID(profile.ID) {
			return nil, fmt.Errorf("ACP agent id %q must be lowercase and contain only letters, numbers, '.', '_' or '-'", profile.ID)
		}
		if _, exists := seen[profile.ID]; exists {
			return nil, fmt.Errorf("duplicate ACP agent id %q", profile.ID)
		}
		seen[profile.ID] = struct{}{}
		if profile.DisplayName == "" {
			profile.DisplayName = profile.ID
		}
		if profile.Command == "" {
			return nil, fmt.Errorf("ACP agent %q command is required", profile.ID)
		}
		profile.Args = trimACPStringList(profile.Args)
		for _, argument := range profile.Args {
			if strings.ContainsRune(argument, '\x00') {
				return nil, fmt.Errorf("ACP agent %q arguments must not contain NUL", profile.ID)
			}
		}
		environment, err := normalizeACPEnvironment(profile.Env, scrubEnvironment)
		if err != nil {
			return nil, fmt.Errorf("ACP agent %q environment: %w", profile.ID, err)
		}
		if len(environment) == 0 {
			environment = nil
		}
		profile.Env = environment
		normalized = append(normalized, cloneACPAgentProfile(profile))
	}
	sort.Slice(normalized, func(i, j int) bool {
		return normalized[i].ID < normalized[j].ID
	})
	return normalized, nil
}

func normalizeACPEnvironment(values []string, scrubValues bool) ([]string, error) {
	seen := make(map[string]struct{}, len(values))
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		entry := value
		if !strings.ContainsRune(entry, '=') {
			entry = strings.TrimSpace(entry)
		}
		if strings.TrimSpace(entry) == "" {
			continue
		}
		key, assignedValue, assigned := strings.Cut(entry, "=")
		key = strings.TrimSpace(key)
		if !validEnvironmentKey(key) {
			return nil, fmt.Errorf("invalid environment variable name %q", key)
		}
		if strings.ContainsRune(entry, '\x00') {
			return nil, fmt.Errorf("environment variable %q contains NUL", key)
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		if scrubValues || !assigned {
			entry = key
		} else {
			entry = key + "=" + assignedValue
		}
		normalized = append(normalized, entry)
	}
	if len(normalized) == 0 {
		return nil, nil
	}
	return normalized, nil
}

func validACPAgentID(id string) bool {
	if id == "" {
		return false
	}
	for index, character := range id {
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' {
			continue
		}
		if index > 0 && (character == '.' || character == '_' || character == '-') {
			continue
		}
		return false
	}
	return true
}

func trimACPStringList(values []string) []string {
	trimmed := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			trimmed = append(trimmed, value)
		}
	}
	return trimmed
}

func cloneACPAgentProfile(profile ACPAgentProfile) ACPAgentProfile {
	profile.Args = slices.Clone(profile.Args)
	profile.Env = slices.Clone(profile.Env)
	return profile
}

func equalACPAgents(left, right []ACPAgentProfile) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].ID != right[index].ID || left[index].DisplayName != right[index].DisplayName || left[index].Command != right[index].Command ||
			!slices.Equal(left[index].Args, right[index].Args) || !slices.Equal(left[index].Env, right[index].Env) {
			return false
		}
	}
	return true
}
