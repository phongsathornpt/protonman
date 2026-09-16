//go:build desktop

package desktop

import (
	"encoding/json"
	"errors"
	"os"
	"slices"
	"strings"

	"github.com/phongsathornpt/protonman/internal/adapter/out/acpclient"
)

const (
	agentProfilesPreferencesKey = "acp.agents.v1"
	defaultAgentID              = "protonman"
	antigravityAgentID          = "antigravity"
	antigravityCommandEnv       = "ANTIGRAVITY_ACP_COMMAND"
	antigravityArgumentsEnv     = "ANTIGRAVITY_ACP_ARGS_JSON"
)

type agentProfile struct {
	ID          string
	DisplayName string
	Command     acpclient.CommandSpec
}

type configuredAgent struct {
	ID          string   `json:"id"`
	DisplayName string   `json:"displayName"`
	Command     string   `json:"command"`
	Args        []string `json:"args"`
	Env         []string `json:"env"`
}

type preferenceReader interface {
	String(string) string
}

func resolveAgentProfilesForPreferences(preferences preferenceReader) ([]agentProfile, error) {
	if raw := strings.TrimSpace(os.Getenv("PROTONMAN_ACP_AGENTS_JSON")); raw != "" {
		return resolveAgentProfilesJSON(raw)
	}
	if preferences != nil {
		if raw := strings.TrimSpace(preferences.String(agentProfilesPreferencesKey)); raw != "" {
			return resolveAgentProfilesJSON(raw)
		}
	}
	return resolveAgentProfiles()
}

func resolveAgentProfiles() ([]agentProfile, error) {
	raw := strings.TrimSpace(os.Getenv("PROTONMAN_ACP_AGENTS_JSON"))
	if raw == "" {
		profile, err := resolveAgentProfile()
		if err != nil {
			return nil, err
		}
		return []agentProfile{profile}, nil
	}
	return resolveAgentProfilesJSON(raw)
}

func resolveAgentProfilesJSON(raw string) ([]agentProfile, error) {
	var configured []configuredAgent
	if err := json.Unmarshal([]byte(raw), &configured); err != nil || len(configured) == 0 {
		return nil, errors.New("PROTONMAN_ACP_AGENTS_JSON must be a non-empty JSON array")
	}
	profiles := make([]agentProfile, 0, len(configured))
	seen := make(map[string]struct{}, len(configured))
	for _, item := range configured {
		id := strings.ToLower(strings.TrimSpace(item.ID))
		command := strings.TrimSpace(item.Command)
		if id == "" || command == "" || item.ID != id {
			return nil, errors.New("ACP agent id and command must be non-empty and trimmed")
		}
		if _, ok := seen[id]; ok {
			return nil, errors.New("duplicate ACP agent id: " + id)
		}
		seen[id] = struct{}{}
		name := strings.TrimSpace(item.DisplayName)
		if name == "" {
			name = id
		}
		profiles = append(profiles, agentProfile{ID: id, DisplayName: name, Command: acpclient.CommandSpec{
			Path: command, Args: append([]string(nil), item.Args...), Env: append([]string(nil), item.Env...),
		}})
	}
	return profiles, nil
}

func configuredAgentsFromProfiles(profiles map[string]agentProfile) []configuredAgent {
	items := make([]configuredAgent, 0, len(profiles))
	for _, profile := range profiles {
		items = append(items, configuredAgent{
			ID: profile.ID, DisplayName: profile.DisplayName, Command: profile.Command.Path,
			Args: append([]string(nil), profile.Command.Args...),
		})
	}
	slices.SortFunc(items, func(a, b configuredAgent) int { return strings.Compare(a.ID, b.ID) })
	return items
}

func (a *application) protonmanExtensionsAvailable() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	agentID := a.activeAgentID
	for _, session := range a.state.Sessions {
		if session.ID == a.state.ActiveSessionID && session.AgentID != "" {
			agentID = session.AgentID
			break
		}
	}
	return agentID == "" || agentID == defaultAgentID
}

func resolveAgentProfile() (agentProfile, error) {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("PROTONMAN_AGENT"))) {
	case "", defaultAgentID:
		return protonmanAgentProfile(), nil
	case antigravityAgentID:
		return antigravityAgentProfile()
	default:
		return agentProfile{}, errors.New("unsupported ACP agent; choose protonman or antigravity")
	}
}

func protonmanAgentProfile() agentProfile {
	return agentProfile{
		ID:          defaultAgentID,
		DisplayName: "Protonman",
		Command: acpclient.CommandSpec{
			Path: resolveACPBinary(),
			Args: []string{"--acp"},
		},
	}
}

func antigravityAgentProfile() (agentProfile, error) {
	command := strings.TrimSpace(os.Getenv(antigravityCommandEnv))
	if command == "" {
		command = "agy_acp_server.par"
	}

	args, err := parseAgentArguments(os.Getenv(antigravityArgumentsEnv))
	if err != nil {
		return agentProfile{}, err
	}
	return agentProfile{
		ID:          antigravityAgentID,
		DisplayName: "Google Antigravity",
		Command:     acpclient.CommandSpec{Path: command, Args: args},
	}, nil
}

func parseAgentArguments(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var args []string
	if err := json.Unmarshal([]byte(raw), &args); err != nil {
		return nil, errors.New("ANTIGRAVITY_ACP_ARGS_JSON must be a JSON string array")
	}
	for _, arg := range args {
		if strings.ContainsRune(arg, '\x00') {
			return nil, errors.New("ACP agent arguments must not contain NUL")
		}
	}
	return args, nil
}
