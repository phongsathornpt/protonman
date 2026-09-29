package app

import (
	"context"
	"slices"
	"strings"
	"sync"
)

// DesktopPreferencesState holds desktop UI preferences such as pinned sessions,
// custom session titles, and active sidebar filter mode.
type DesktopPreferencesState struct {
	PinnedSessions       []string            `json:"pinnedSessions,omitempty"`
	CustomTitles         map[string]string   `json:"customTitles,omitempty"`
	FilterMode           string              `json:"filterMode,omitempty"`
	Theme                string              `json:"theme,omitempty"`
	AgentDefaultModels   map[string]string   `json:"agentDefaultModels,omitempty"`
	AgentAvailableModels map[string][]string `json:"agentAvailableModels,omitempty"`
}

// DesktopPreferencesRepository is the outbound port for persisting desktop preferences.
type DesktopPreferencesRepository interface {
	Load(context.Context) (DesktopPreferencesState, error)
	Save(context.Context, DesktopPreferencesState) error
}

// DesktopPreferences provides thread-safe in-memory caching and persistent updates
// for desktop client preferences.
type DesktopPreferences struct {
	repository DesktopPreferencesRepository
	mu         sync.RWMutex
	state      DesktopPreferencesState
	loaded     bool
}

func NewDesktopPreferences(repository DesktopPreferencesRepository) *DesktopPreferences {
	return &DesktopPreferences{
		repository: repository,
		state: DesktopPreferencesState{
			PinnedSessions:       make([]string, 0),
			CustomTitles:         make(map[string]string),
			FilterMode:           "all",
			Theme:                "system",
			AgentDefaultModels:   make(map[string]string),
			AgentAvailableModels: make(map[string][]string),
		},
	}
}

func (p *DesktopPreferences) Available() bool {
	return p != nil && p.repository != nil
}

// Load ensures preferences are loaded from the repository into memory.
func (p *DesktopPreferences) Load(ctx context.Context) (DesktopPreferencesState, error) {
	if p == nil {
		return DesktopPreferencesState{}, nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.loaded || p.repository == nil {
		return p.cloneStateLocked(), nil
	}
	state, err := p.repository.Load(ctx)
	if err != nil {
		return p.cloneStateLocked(), err
	}
	if state.CustomTitles == nil {
		state.CustomTitles = make(map[string]string)
	}
	if state.AgentDefaultModels == nil {
		state.AgentDefaultModels = make(map[string]string)
	}
	if state.FilterMode == "" {
		state.FilterMode = "all"
	}
	p.state = state
	p.loaded = true
	return p.cloneStateLocked(), nil
}

// Snapshot returns the current in-memory preferences without blocking on I/O.
func (p *DesktopPreferences) Snapshot() DesktopPreferencesState {
	if p == nil {
		return DesktopPreferencesState{}
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.cloneStateLocked()
}

// TogglePin toggles the pinned status of a session and saves the change.
func (p *DesktopPreferences) TogglePin(ctx context.Context, sessionID string) (bool, error) {
	if p == nil {
		return false, nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return false, nil
	}

	p.mu.Lock()
	var pinned bool
	idx := slices.Index(p.state.PinnedSessions, sessionID)
	if idx >= 0 {
		p.state.PinnedSessions = slices.Delete(p.state.PinnedSessions, idx, idx+1)
		pinned = false
	} else {
		p.state.PinnedSessions = append(p.state.PinnedSessions, sessionID)
		pinned = true
	}
	cloned := p.cloneStateLocked()
	repo := p.repository
	p.mu.Unlock()

	if repo != nil {
		if err := repo.Save(ctx, cloned); err != nil {
			return pinned, err
		}
	}
	return pinned, nil
}

// SetCustomTitle updates or deletes a custom title for a session and saves the change.
func (p *DesktopPreferences) SetCustomTitle(ctx context.Context, sessionID, title string) error {
	if p == nil {
		return nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	title = strings.TrimSpace(title)

	p.mu.Lock()
	if p.state.CustomTitles == nil {
		p.state.CustomTitles = make(map[string]string)
	}
	if title == "" {
		delete(p.state.CustomTitles, sessionID)
	} else {
		p.state.CustomTitles[sessionID] = title
	}
	cloned := p.cloneStateLocked()
	repo := p.repository
	p.mu.Unlock()

	if repo != nil {
		return repo.Save(ctx, cloned)
	}
	return nil
}

// SetFilterMode updates the current filter mode (all, running, pinned) and saves the change.
func (p *DesktopPreferences) SetFilterMode(ctx context.Context, mode string) error {
	if p == nil {
		return nil
	}
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode != "running" && mode != "pinned" {
		mode = "all"
	}

	p.mu.Lock()
	p.state.FilterMode = mode
	cloned := p.cloneStateLocked()
	repo := p.repository
	p.mu.Unlock()

	if repo != nil {
		return repo.Save(ctx, cloned)
	}
	return nil
}

// SetTheme updates the UI theme (dark, light, slate-dark, slate-light) and saves the change.
func (p *DesktopPreferences) SetTheme(ctx context.Context, theme string) error {
	if p == nil {
		return nil
	}
	theme = strings.ToLower(strings.TrimSpace(theme))

	p.mu.Lock()
	p.state.Theme = theme
	cloned := p.cloneStateLocked()
	repo := p.repository
	p.mu.Unlock()

	if repo != nil {
		return repo.Save(ctx, cloned)
	}
	return nil
}

// RemoveSession cleans up any pinned state or custom title when a session is deleted.
func (p *DesktopPreferences) RemoveSession(ctx context.Context, sessionID string) error {
	if p == nil {
		return nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}

	p.mu.Lock()
	changed := false
	if idx := slices.Index(p.state.PinnedSessions, sessionID); idx >= 0 {
		p.state.PinnedSessions = slices.Delete(p.state.PinnedSessions, idx, idx+1)
		changed = true
	}
	if p.state.CustomTitles != nil {
		if _, exists := p.state.CustomTitles[sessionID]; exists {
			delete(p.state.CustomTitles, sessionID)
			changed = true
		}
	}
	if !changed {
		p.mu.Unlock()
		return nil
	}
	cloned := p.cloneStateLocked()
	repo := p.repository
	p.mu.Unlock()

	if repo != nil {
		return repo.Save(ctx, cloned)
	}
	return nil
}

// SetAgentDefaultModel updates the preferred default model for an agent and saves the change.
func (p *DesktopPreferences) SetAgentDefaultModel(ctx context.Context, agentID, model string) error {
	if p == nil {
		return nil
	}
	agentID = strings.TrimSpace(agentID)
	model = strings.TrimSpace(model)
	if agentID == "" || model == "" {
		return nil
	}

	p.mu.Lock()
	if p.state.AgentDefaultModels == nil {
		p.state.AgentDefaultModels = make(map[string]string)
	}
	p.state.AgentDefaultModels[agentID] = model
	cloned := p.cloneStateLocked()
	repo := p.repository
	p.mu.Unlock()

	if repo != nil {
		return repo.Save(ctx, cloned)
	}
	return nil
}

// SetAgentAvailableModels updates the cached available models for an agent and saves the change.
func (p *DesktopPreferences) SetAgentAvailableModels(ctx context.Context, agentID string, models []string) error {
	if p == nil {
		return nil
	}
	agentID = strings.TrimSpace(agentID)
	if agentID == "" || len(models) == 0 {
		return nil
	}

	p.mu.Lock()
	if p.state.AgentAvailableModels == nil {
		p.state.AgentAvailableModels = make(map[string][]string)
	}
	p.state.AgentAvailableModels[agentID] = slices.Clone(models)
	cloned := p.cloneStateLocked()
	repo := p.repository
	p.mu.Unlock()

	if repo != nil {
		return repo.Save(ctx, cloned)
	}
	return nil
}

func (p *DesktopPreferences) cloneStateLocked() DesktopPreferencesState {
	cloned := DesktopPreferencesState{
		PinnedSessions:       append([]string(nil), p.state.PinnedSessions...),
		CustomTitles:         make(map[string]string, len(p.state.CustomTitles)),
		FilterMode:           p.state.FilterMode,
		Theme:                p.state.Theme,
		AgentDefaultModels:   make(map[string]string, len(p.state.AgentDefaultModels)),
		AgentAvailableModels: make(map[string][]string, len(p.state.AgentAvailableModels)),
	}
	for k, v := range p.state.CustomTitles {
		cloned.CustomTitles[k] = v
	}
	for k, v := range p.state.AgentDefaultModels {
		cloned.AgentDefaultModels[k] = v
	}
	for k, v := range p.state.AgentAvailableModels {
		cloned.AgentAvailableModels[k] = slices.Clone(v)
	}
	if cloned.FilterMode == "" {
		cloned.FilterMode = "all"
	}
	return cloned
}
