package agent

import (
	"time"

	"github.com/projectTHORN/proton/internal/core/permission"
	"github.com/projectTHORN/proton/internal/core/tool"
	"github.com/projectTHORN/proton/internal/engine/toolcall"
	sdk "github.com/projectTHORN/proton/proton-sdk"
)

// SetEnabled controls whether new subagents may be spawned. It never cancels existing agents.
// The coordinator lock linearizes capability changes with Spawn admission: once
// disabling returns, no later admission can have observed the previous state.
func (c *Coordinator) SetEnabled(enabled bool) {
	if c == nil {
		return
	}
	c.agentsMu.Lock()
	c.enabled.Store(enabled)
	c.agentsMu.Unlock()
}

// Enabled reports whether new subagents may be spawned.
func (c *Coordinator) Enabled() bool { return c != nil && c.enabled.Load() }

// HasAgents reports whether any live or retained subagent lifecycle record is
// still queryable. It prunes expired terminal records without allocating or
// sorting a snapshot, making it suitable for capability publication checks.
func (c *Coordinator) HasAgents() bool {
	if c == nil {
		return false
	}
	c.agentsMu.Lock()
	defer c.agentsMu.Unlock()
	c.pruneExpiredLocked(time.Now())
	return len(c.agents) > 0
}

func (c *Coordinator) Close() error {
	c.agentsMu.Lock()
	c.closed.Store(true)
	c.rootStop()
	for _, entry := range c.agents {
		if !entry.status.State.Terminal() {
			entry.cancel()
		}
	}
	c.agentsMu.Unlock()
	c.closeSubscribers()

	c.closeOnce.Do(func() {
		go func() { c.wg.Wait(); close(c.closeDone) }()
	})
	deadline := time.NewTimer(c.closeTimeout)
	defer deadline.Stop()
	select {
	case <-c.closeDone:
		return nil
	case <-deadline.C:
		c.agentsMu.RLock()
		remaining := 0
		for _, entry := range c.agents {
			if !entry.status.State.Terminal() {
				remaining++
			}
		}
		c.agentsMu.RUnlock()
		return &ShutdownTimeoutError{ActiveAgents: remaining}
	}
}

// SetParentRegistry sets or updates the parent tool registry for scoping subagent tools.
func (c *Coordinator) SetParentRegistry(registry tool.Registry) {
	c.agentsMu.Lock()
	defer c.agentsMu.Unlock()
	c.parentRegistry = registry
}

// SetLanguageModel dynamically updates the proton-sdk model used by child subagents.
func (c *Coordinator) SetLanguageModel(languageModel sdk.LanguageModel) {
	c.agentsMu.Lock()
	defer c.agentsMu.Unlock()
	c.languageModel = languageModel
}

// SetModelResolver updates immutable per-profile model overrides for future admissions.
// Already admitted agents keep the model snapshot bound at Spawn time.
func (c *Coordinator) SetModelResolver(resolver *ModelResolver) {
	if c == nil {
		return
	}
	c.agentsMu.Lock()
	c.modelResolver = resolver
	c.agentsMu.Unlock()
}

// LanguageModel returns the proton-sdk model used by child subagents.
func (c *Coordinator) LanguageModel() sdk.LanguageModel {
	c.agentsMu.RLock()
	defer c.agentsMu.RUnlock()
	return c.languageModel
}

// SetPermissionMode dynamically updates the permission mode for subagents.
func (c *Coordinator) SetPermissionMode(mode permission.Mode) {
	c.agentsMu.Lock()
	defer c.agentsMu.Unlock()
	if mode.Valid() {
		c.permissionMode = mode
	}
}

// PermissionMode returns the active permission mode configured for subagents.
func (c *Coordinator) PermissionMode() permission.Mode {
	c.agentsMu.RLock()
	defer c.agentsMu.RUnlock()
	return c.permissionMode
}

// SetPrompt dynamically updates the interactive permission prompt resolver for subagents.
func (c *Coordinator) SetPrompt(prompt toolcall.PermissionPrompt) {
	c.agentsMu.Lock()
	defer c.agentsMu.Unlock()
	c.prompt = prompt
}

// SetCallGuard dynamically updates the execution guard (e.g. plan mode) for subagents.
func (c *Coordinator) SetCallGuard(guard toolcall.CallGuard) {
	c.agentsMu.Lock()
	defer c.agentsMu.Unlock()
	c.guard = guard
}

// CallGuard returns the active call guard for subagents.
func (c *Coordinator) CallGuard() toolcall.CallGuard {
	c.agentsMu.RLock()
	defer c.agentsMu.RUnlock()
	return c.guard
}

// SetReasoningResolver updates immutable per-profile reasoning overrides for future admissions.
// Already admitted agents keep the reasoning snapshot bound at Spawn time.
func (c *Coordinator) SetReasoningResolver(resolver *ReasoningResolver) {
	if c == nil {
		return
	}
	c.agentsMu.Lock()
	c.reasoningResolver = resolver
	c.agentsMu.Unlock()
}

// SetReasoningEffort updates the explicit reasoning override inherited by new subagent turns.
func (c *Coordinator) SetReasoningEffort(effort sdk.ReasoningEffort) {
	if !effort.Valid() {
		return
	}
	c.agentsMu.Lock()
	c.reasoningEffort = effort
	c.agentsMu.Unlock()
}

// ReasoningEffort returns the explicit reasoning override inherited by new subagents.
func (c *Coordinator) ReasoningEffort() sdk.ReasoningEffort {
	c.agentsMu.RLock()
	defer c.agentsMu.RUnlock()
	return c.reasoningEffort
}
