package agent

import (
	"fmt"
	"time"

	"github.com/projectTHORN/proton/internal/permission"
	"github.com/projectTHORN/proton/internal/tool"
	"github.com/projectTHORN/proton/internal/toolcall"
	sdk "github.com/projectTHORN/proton/proton-sdk"
)

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
		return fmt.Errorf("coordinator close timed out with %d active subagent(s)", remaining)
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
