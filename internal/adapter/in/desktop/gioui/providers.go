//go:build desktop || desktop_gio

package gioui

import (
	"context"
	"slices"
	"strings"
	"time"

	desktopstate "github.com/phongsathornpt/protonman/internal/feature/desktop"
)

type acpProviderInfo struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Protocol     string `json:"protocol"`
	BaseURL      string `json:"baseURL"`
	RequiresKey  bool   `json:"requiresKey"`
	IsConfigured bool   `json:"isConfigured"`
	IsActive     bool   `json:"isActive"`
	IsFree       bool   `json:"isFree"`
	HasKey       bool   `json:"hasKey"`
	DefaultModel string `json:"defaultModel,omitempty"`
}

type acpProvidersListResult struct {
	ActiveProvider string            `json:"activeProvider"`
	ActiveModel    string            `json:"activeModel"`
	Providers      []acpProviderInfo `json:"providers"`
}

type acpProvidersSaveParams struct {
	ProviderName string `json:"providerName"`
	ProviderType string `json:"providerType"`
	PreviousName string `json:"previousName,omitempty"`
	BaseURL      string `json:"baseURL"`
	APIKey       string `json:"apiKey,omitempty"`
	DefaultModel string `json:"defaultModel,omitempty"`
	Activate     bool   `json:"activate"`
}

type acpProvidersSaveResult struct {
	Success        bool   `json:"success"`
	ActiveProvider string `json:"activeProvider,omitempty"`
	ActiveModel    string `json:"activeModel,omitempty"`
}

type acpProvidersModelsParams struct {
	ProviderName string `json:"providerName"`
	ProviderType string `json:"providerType,omitempty"`
	BaseURL      string `json:"baseURL,omitempty"`
	APIKey       string `json:"apiKey,omitempty"`
}

type acpProvidersModelsResult struct {
	ProviderName string `json:"providerName"`
	Models       []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"models"`
}

func (c *controller) refreshProviders() {
	c.mu.RLock()
	client := c.clients[controllerAgentID]
	c.mu.RUnlock()
	if client == nil {
		return
	}

	callCtx, cancel := context.WithTimeout(c.ctx, 5*time.Second)
	defer cancel()

	var result acpProvidersListResult
	if err := client.Call(callCtx, "protonman/providers/list", map[string]any{}, &result); err != nil {
		return
	}

	c.mu.Lock()
	providers := make([]desktopstate.ProviderState, 0, len(result.Providers))
	for _, p := range result.Providers {
		providers = append(providers, desktopstate.ProviderState{
			ID:           strings.TrimSpace(p.ID),
			Name:         strings.TrimSpace(p.Name),
			Protocol:     strings.TrimSpace(p.Protocol),
			BaseURL:      strings.TrimSpace(p.BaseURL),
			RequiresKey:  p.RequiresKey,
			IsConfigured: p.IsConfigured,
			IsActive:     p.IsActive,
			IsFree:       p.IsFree,
			HasKey:       p.HasKey,
			DefaultModel: strings.TrimSpace(p.DefaultModel),
		})
	}
	c.activeProvider = strings.TrimSpace(result.ActiveProvider)
	c.activeModel = strings.TrimSpace(result.ActiveModel)
	desktopstate.Apply(&c.state, desktopstate.Event{
		Kind:      desktopstate.EventProvidersUpdated,
		Providers: providers,
	})
	c.revision++
	c.mu.Unlock()
	c.notify()
}

func (c *controller) saveProvider(params acpProvidersSaveParams, onDone func(err error)) {
	c.mu.Lock()
	client := c.clients[controllerAgentID]
	if client == nil {
		c.mu.Unlock()
		if onDone != nil {
			onDone(context.Canceled)
		}
		return
	}
	c.providerUpdating = true
	c.providerError = ""
	c.revision++
	c.mu.Unlock()
	c.notify()

	go func() {
		defer func() {
			c.mu.Lock()
			c.providerUpdating = false
			c.revision++
			c.mu.Unlock()
			c.notify()
		}()

		callCtx, cancel := context.WithTimeout(c.ctx, 10*time.Second)
		defer cancel()

		var result acpProvidersSaveResult
		err := client.Call(callCtx, "protonman/providers/save", params, &result)
		if err != nil {
			c.mu.Lock()
			c.providerError = compactError(err)
			c.revision++
			c.mu.Unlock()
			c.notify()
			if onDone != nil {
				onDone(err)
			}
			return
		}

		c.refreshProviders()

		if params.Activate && strings.TrimSpace(params.DefaultModel) != "" {
			c.setRuntimeModel(params.ProviderName, params.DefaultModel)
		}

		if onDone != nil {
			onDone(nil)
		}
	}()
}

func (c *controller) deleteProvider(providerName string, onDone func(err error)) {
	c.mu.Lock()
	client := c.clients[controllerAgentID]
	if client == nil {
		c.mu.Unlock()
		if onDone != nil {
			onDone(context.Canceled)
		}
		return
	}
	c.providerUpdating = true
	c.providerError = ""
	c.revision++
	c.mu.Unlock()
	c.notify()

	go func() {
		defer func() {
			c.mu.Lock()
			c.providerUpdating = false
			c.revision++
			c.mu.Unlock()
			c.notify()
		}()

		callCtx, cancel := context.WithTimeout(c.ctx, 5*time.Second)
		defer cancel()

		var result struct {
			Success bool `json:"success"`
		}
		err := client.Call(callCtx, "protonman/providers/delete", map[string]any{
			"providerName": providerName,
		}, &result)
		if err != nil {
			c.mu.Lock()
			c.providerError = compactError(err)
			c.revision++
			c.mu.Unlock()
			c.notify()
			if onDone != nil {
				onDone(err)
			}
			return
		}

		c.mu.Lock()
		delete(c.providerModelsCache, strings.ToLower(providerName))
		c.revision++
		c.mu.Unlock()

		c.refreshProviders()
		if onDone != nil {
			onDone(nil)
		}
	}()
}

func (c *controller) fetchProviderModels(providerID, baseURL, apiKey, providerType string, onDone func(models []string, err error)) {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return
	}
	c.mu.Lock()
	client := c.clients[controllerAgentID]
	if client == nil {
		c.mu.Unlock()
		if onDone != nil {
			onDone(nil, context.Canceled)
		}
		return
	}
	if c.providerModelsLoading == nil {
		c.providerModelsLoading = make(map[string]bool)
	}
	c.providerModelsLoading[providerID] = true
	c.revision++
	c.mu.Unlock()
	c.notify()

	go func() {
		defer func() {
			c.mu.Lock()
			delete(c.providerModelsLoading, providerID)
			c.revision++
			c.mu.Unlock()
			c.notify()
		}()

		callCtx, cancel := context.WithTimeout(c.ctx, 8*time.Second)
		defer cancel()

		var result acpProvidersModelsResult
		err := client.Call(callCtx, "protonman/providers/models", acpProvidersModelsParams{
			ProviderName: providerID,
			ProviderType: providerType,
			BaseURL:      baseURL,
			APIKey:       apiKey,
		}, &result)

		if err != nil {
			if onDone != nil {
				onDone(nil, err)
			}
			return
		}

		models := make([]string, 0, len(result.Models))
		for _, m := range result.Models {
			id := strings.TrimSpace(m.ID)
			if id != "" {
				models = append(models, id)
			}
		}

		c.mu.Lock()
		if c.providerModelsCache == nil {
			c.providerModelsCache = make(map[string][]string)
		}
		c.providerModelsCache[strings.ToLower(providerID)] = slices.Clone(models)
		c.revision++
		c.mu.Unlock()
		c.notify()

		if onDone != nil {
			onDone(models, nil)
		}
	}()
}

func cloneProviderModels(src map[string][]string) map[string][]string {
	if src == nil {
		return nil
	}
	out := make(map[string][]string, len(src))
	for k, v := range src {
		out[k] = slices.Clone(v)
	}
	return out
}

func cloneBoolMap(src map[string]bool) map[string]bool {
	if src == nil {
		return nil
	}
	out := make(map[string]bool, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}
