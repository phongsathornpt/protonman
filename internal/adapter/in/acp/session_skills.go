package acp

import (
	"context"
	"fmt"
	"strings"

	"github.com/phongsathornpt/protonman/internal/feature/skill"
)

const (
	methodSessionSkills       = "protonman/session/skills"
	methodSessionSkillsToggle = "protonman/session/skills/toggle"
)

// SkillSaver persists active skills to project/user config.
type SkillSaver func(workDir string, activeSkills []string) error

// WithSkills configures the skill registry and persistence callback for ACP.
func WithSkills(registry *skill.Registry, saver SkillSaver) Option {
	return func(server *Server) {
		server.skillRegistry = registry
		server.skillSaver = saver
	}
}

type ProtonmanSessionSkillsParams struct {
	SessionID string `json:"sessionId"`
}

type ProtonmanSkillEntry struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Scope       string   `json:"scope"`
	Active      bool     `json:"active"`
	Locked      bool     `json:"locked,omitempty"`
	LockStatus  string   `json:"lockStatus,omitempty"`
	Resources   []string `json:"resources,omitempty"`
}

type ProtonmanSessionSkillsResult struct {
	SessionID string                `json:"sessionId"`
	Skills    []ProtonmanSkillEntry `json:"skills"`
}

type ProtonmanSessionSkillToggleParams struct {
	SessionID string `json:"sessionId"`
	Name      string `json:"name"`
}

type ProtonmanSessionSkillToggleResult struct {
	SessionID string `json:"sessionId"`
	Name      string `json:"name"`
	Active    bool   `json:"active"`
}

func (s *Server) sessionSkills(ctx context.Context, sessionID string) (ProtonmanSessionSkillsResult, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return ProtonmanSessionSkillsResult{}, fmt.Errorf("sessionId is required")
	}
	if s.skillRegistry == nil {
		return ProtonmanSessionSkillsResult{
			SessionID: sessionID,
			Skills:    []ProtonmanSkillEntry{},
		}, nil
	}

	skills := s.skillRegistry.List()
	entries := make([]ProtonmanSkillEntry, 0, len(skills))
	for _, item := range skills {
		entries = append(entries, ProtonmanSkillEntry{
			Name:        item.Name,
			Description: item.Description,
			Scope:       string(item.Scope),
			Active:      s.skillRegistry.IsActivated(item.Name),
			Locked:      item.Locked,
			LockStatus:  item.LockStatus,
			Resources:   item.Resources,
		})
	}

	return ProtonmanSessionSkillsResult{
		SessionID: sessionID,
		Skills:    entries,
	}, nil
}

func (s *Server) sessionSkillToggle(ctx context.Context, params ProtonmanSessionSkillToggleParams) (ProtonmanSessionSkillToggleResult, error) {
	sessionID := strings.TrimSpace(params.SessionID)
	if sessionID == "" {
		return ProtonmanSessionSkillToggleResult{}, fmt.Errorf("sessionId is required")
	}
	skillName := strings.TrimSpace(params.Name)
	if skillName == "" {
		return ProtonmanSessionSkillToggleResult{}, fmt.Errorf("skill name is required")
	}
	if s.skillRegistry == nil {
		return ProtonmanSessionSkillToggleResult{}, fmt.Errorf("skill registry is unavailable")
	}

	active, err := s.skillRegistry.Toggle(skillName)
	if err != nil {
		return ProtonmanSessionSkillToggleResult{}, err
	}

	workDir := ""
	if sess, ok := s.lookupSession(sessionID); ok {
		workDir = sess.cwd
		_ = sess.saveStateDetached(ctx)
	}

	if s.skillSaver != nil {
		if saveErr := s.skillSaver(workDir, s.skillRegistry.ActivatedList()); saveErr != nil {
			return ProtonmanSessionSkillToggleResult{}, fmt.Errorf("save active skills: %w", saveErr)
		}
	}

	return ProtonmanSessionSkillToggleResult{
		SessionID: sessionID,
		Name:      skillName,
		Active:    active,
	}, nil
}
