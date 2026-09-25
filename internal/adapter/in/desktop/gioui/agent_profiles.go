//go:build desktop || desktop_gio

package gioui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/phongsathornpt/protonman/internal/adapter/out/acpclient"
	"github.com/phongsathornpt/protonman/internal/app"
	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

func defaultACPAgentProfile() app.ACPAgentProfile {
	return app.ACPAgentProfile{
		ID:          controllerAgentID,
		DisplayName: "Protonman",
		Command:     resolveACPBinary(),
		Args:        []string{"--acp"},
	}
}

func cloneACPAgentProfile(profile app.ACPAgentProfile) app.ACPAgentProfile {
	profile.Args = slices.Clone(profile.Args)
	profile.Env = slices.Clone(profile.Env)
	return profile
}

func cloneACPAgentProfiles(profiles map[string]app.ACPAgentProfile) []app.ACPAgentProfile {
	cloned := make([]app.ACPAgentProfile, 0, len(profiles))
	for _, profile := range profiles {
		cloned = append(cloned, cloneACPAgentProfile(profile))
	}
	sort.Slice(cloned, func(left, right int) bool {
		return cloned[left].ID < cloned[right].ID
	})
	return cloned
}

func preferredAgentID(profiles []app.ACPAgentProfile) string {
	for _, profile := range profiles {
		if profile.ID == controllerAgentID {
			return profile.ID
		}
	}
	if len(profiles) > 0 {
		return profiles[0].ID
	}
	return ""
}

func commandSpecForACPAgent(profile app.ACPAgentProfile) acpclient.CommandSpec {
	spec := acpclient.CommandSpec{
		Path: strings.TrimSpace(profile.Command),
		Args: slices.Clone(profile.Args),
	}
	for _, key := range profile.Env {
		if strings.ContainsRune(key, '=') {
			spec.Env = append(spec.Env, key)
			continue
		}
		if value, ok := os.LookupEnv(key); ok {
			spec.Env = append(spec.Env, key+"="+value)
		}
	}
	return spec
}

func (c *controller) lockAgentSession(agentID string) func() {
	value, _ := c.sessionLocks.LoadOrStore(agentID, &sync.Mutex{})
	lock := value.(*sync.Mutex)
	lock.Lock()
	return lock.Unlock
}

func (c *controller) sortedProfiles() []app.ACPAgentProfile {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return cloneACPAgentProfiles(c.profiles)
}

func (c *controller) selectAgent(agentID string) {
	agentID = strings.ToLower(strings.TrimSpace(agentID))
	c.mu.Lock()
	if _, ok := c.profiles[agentID]; !ok {
		c.mu.Unlock()
		return
	}
	c.activeAgentID = agentID
	c.setProjectDefaultAgentLocked(c.state.ActiveProjectID, agentID)
	c.statuses[agentID] = "Selected · " + c.profiles[agentID].DisplayName
	c.revision++
	c.mu.Unlock()
	c.notify()
}

func (c *controller) selectProject(projectID string) {
	projectID = strings.TrimSpace(projectID)
	c.mu.Lock()
	project, ok := projectByID(c.state, projectID)
	if !ok {
		c.mu.Unlock()
		return
	}
	c.state.ActiveProjectID = project.ID
	c.state.ActiveSessionID = ""
	agentID := c.agentForProjectLocked(project.ID)
	if agentID != "" {
		c.activeAgentID = agentID
		c.statuses[agentID] = "Project selected · " + project.Name
	}
	c.revision++
	c.mu.Unlock()
	c.notify()
}

func (c *controller) agentForProjectLocked(projectID string) string {
	project, ok := projectByID(c.state, projectID)
	if !ok {
		return c.activeAgentID
	}
	if _, exists := c.profiles[project.DefaultAgentID]; project.DefaultAgentID != "" && exists {
		return project.DefaultAgentID
	}
	for _, agentID := range project.AgentIDs {
		if _, exists := c.profiles[agentID]; exists {
			return agentID
		}
	}
	return c.activeAgentID
}

func (c *controller) setProjectDefaultAgentLocked(projectID, agentID string) {
	agentID = strings.TrimSpace(agentID)
	if projectID == "" || agentID == "" {
		return
	}
	for index := range c.state.Projects {
		project := &c.state.Projects[index]
		if project.ID != projectID {
			continue
		}
		if !slices.Contains(project.AgentIDs, agentID) {
			project.AgentIDs = append(project.AgentIDs, agentID)
			sort.Strings(project.AgentIDs)
		}
		project.DefaultAgentID = agentID
		return
	}
}

func projectByID(state desktopstate.State, projectID string) (desktopstate.ProjectState, bool) {
	for _, project := range state.Projects {
		if project.ID == projectID {
			return project, true
		}
	}
	return desktopstate.ProjectState{}, false
}

func (c *controller) clientForSessionLocked(sessionID string) (*acpclient.Client, string) {
	session, ok := desktopSessionByID(c.state, sessionID)
	if !ok {
		return nil, ""
	}
	agentID := strings.TrimSpace(session.AgentID)
	if agentID == "" {
		agentID = c.activeAgentID
	}
	return c.clients[agentID], agentID
}

func (c *controller) clientCurrentLocked(agentID string, client *acpclient.Client) bool {
	return client != nil && c.clients[agentID] == client
}

func (c *controller) agentIDForClientLocked(client *acpclient.Client) string {
	if client == nil {
		return ""
	}
	for agentID, current := range c.clients {
		if current == client {
			return agentID
		}
	}
	return ""
}

func (c *controller) saveAgentProfile(originalID, id, displayName, command, argsJSON, envJSON string) {
	profile, err := acpaentProfileFromEditor(id, displayName, command, argsJSON, envJSON)
	if err != nil {
		c.setStatus("Invalid ACP agent · " + compactError(err))
		return
	}
	originalID = strings.ToLower(strings.TrimSpace(originalID))
	c.mu.Lock()
	if c.agentMutation {
		c.mu.Unlock()
		c.setStatus("ACP agent settings update already in progress")
		return
	}
	if !c.agentProfiles.Available() {
		c.mu.Unlock()
		c.setStatus("ACP agent settings repository is unavailable")
		return
	}
	if originalID != profile.ID {
		if _, exists := c.profiles[profile.ID]; exists {
			c.mu.Unlock()
			c.setStatus("ACP agent id already exists · " + profile.ID)
			return
		}
	}
	next := make(map[string]app.ACPAgentProfile, len(c.profiles)+1)
	for key, existing := range c.profiles {
		next[key] = cloneACPAgentProfile(existing)
	}
	if originalID != "" && originalID != profile.ID {
		delete(next, originalID)
	}
	next[profile.ID] = profile
	c.agentMutation = true
	c.statuses[c.activeAgentID] = "Saving ACP agent settings…"
	c.revision++
	c.mu.Unlock()
	c.notify()

	go c.persistAgentProfiles(next, profile, "ACP agent saved · restart Desktop to apply")
}

func (c *controller) removeAgentProfile(agentID string) {
	agentID = strings.ToLower(strings.TrimSpace(agentID))
	c.mu.Lock()
	if c.agentMutation {
		c.mu.Unlock()
		c.setStatus("ACP agent settings update already in progress")
		return
	}
	if !c.agentProfiles.Available() {
		c.mu.Unlock()
		c.setStatus("ACP agent settings repository is unavailable")
		return
	}
	if len(c.profiles) <= 1 {
		c.mu.Unlock()
		c.setStatus("Keep at least one ACP agent configured")
		return
	}
	if _, exists := c.profiles[agentID]; !exists {
		c.mu.Unlock()
		c.setStatus("ACP agent not found · " + agentID)
		return
	}
	next := make(map[string]app.ACPAgentProfile, len(c.profiles)-1)
	for key, profile := range c.profiles {
		if key != agentID {
			next[key] = cloneACPAgentProfile(profile)
		}
	}
	c.agentMutation = true
	c.statuses[c.activeAgentID] = "Removing ACP agent…"
	c.revision++
	c.mu.Unlock()
	c.notify()

	go c.persistAgentProfiles(next, app.ACPAgentProfile{}, "ACP agent removed · restart Desktop to apply")
}

func (c *controller) persistAgentProfiles(next map[string]app.ACPAgentProfile, changed app.ACPAgentProfile, successStatus string) {
	ctx := c.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	items := cloneACPAgentProfiles(next)
	err := c.agentProfiles.Save(ctx, items)

	c.mu.Lock()
	c.agentMutation = false
	if err != nil {
		c.agentError = compactError(err)
		c.statuses[c.activeAgentID] = "ACP agent settings save failed · " + c.agentError
	} else {
		if !c.agentConfigOverridden {
			c.agentError = ""
		}
		c.applyAgentProfilesLocked(next, changed, successStatus)
	}
	c.revision++
	c.mu.Unlock()
	c.notify()
}

func (c *controller) applyAgentProfilesLocked(next map[string]app.ACPAgentProfile, changed app.ACPAgentProfile, successStatus string) {
	previous := c.profiles
	c.profiles = make(map[string]app.ACPAgentProfile, len(next))
	for id, profile := range next {
		c.profiles[id] = cloneACPAgentProfile(profile)
		before, existed := previous[id]
		switch {
		case !existed:
			c.connections[id] = connectionReconnecting
			c.statuses[id] = "Restart required to connect"
		case changed.ID == id && !equalDesktopAgentProfile(before, profile):
			c.statuses[id] = "Restart required to apply"
		}
	}
	if _, exists := c.profiles[c.activeAgentID]; !exists {
		c.activeAgentID = preferredAgentID(cloneACPAgentProfiles(next))
	}
	active := c.activeAgentID
	if _, exists := c.profiles[changed.ID]; changed.ID != "" && exists && c.activeAgentID == "" {
		active = changed.ID
	}
	c.statuses[active] = successStatus
}

func acpaentProfileFromEditor(id, displayName, command, argsJSON, envJSON string) (app.ACPAgentProfile, error) {
	profile := app.ACPAgentProfile{
		ID:          strings.ToLower(strings.TrimSpace(id)),
		DisplayName: strings.TrimSpace(displayName),
		Command:     strings.TrimSpace(command),
	}
	args, err := parseMCPStringList(argsJSON)
	if err != nil {
		return app.ACPAgentProfile{}, fmt.Errorf("arguments: %w", err)
	}
	environment, err := parseMCPStringList(envJSON)
	if err != nil {
		return app.ACPAgentProfile{}, fmt.Errorf("environment: %w", err)
	}
	profile.Args = args
	profile.Env = environment
	encoded, err := json.Marshal([]app.ACPAgentProfile{profile})
	if err != nil {
		return app.ACPAgentProfile{}, err
	}
	profiles, err := app.ParseACPAgentsJSON(string(encoded))
	if err != nil {
		return app.ACPAgentProfile{}, err
	}
	return profiles[0], nil
}

func equalDesktopAgentProfile(left, right app.ACPAgentProfile) bool {
	return left.ID == right.ID && left.DisplayName == right.DisplayName && left.Command == right.Command &&
		slices.Equal(left.Args, right.Args) && slices.Equal(left.Env, right.Env)
}
