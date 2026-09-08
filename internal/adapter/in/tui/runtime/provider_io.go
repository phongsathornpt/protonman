package runtime

import (
	"context"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/phongsathornpt/protonman/internal/adapter/out/config"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/base/runtimepolicy"
)

type modelsFetchedMsg struct {
	providerName string
	baseURL      string
	apiKey       string
	models       []model.RemoteModel
	requestID    uint64
	err          error
}

type providerSavedMsg struct {
	providerName string
	providerType string
	previousName string
	baseURL      string
	apiKey       string
	modelID      string
	activated    bool
	err          error
}

type providerFetchRequest struct {
	ctx              context.Context
	requestID        uint64
	providerName     string
	providerType     string
	baseURL          string
	apiKey           string
	discoveryTimeout time.Duration
}

func (v *providerPaneView) beginFetch(parent context.Context, timeouts ...time.Duration) tea.Cmd {
	discoveryTimeout := runtimepolicy.ModelDiscoveryTimeout
	if len(timeouts) > 0 && timeouts[0] > 0 {
		discoveryTimeout = timeouts[0]
	}
	if v.fetchCancel != nil {
		v.fetchCancel()
	}
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	v.fetchCancel = cancel
	v.fetchRequestID++
	v.state = providerStateFetching
	return fetchProviderModelsCmd(providerFetchRequest{
		ctx:              ctx,
		requestID:        v.fetchRequestID,
		providerName:     strings.TrimSpace(v.nameInput.Value()),
		providerType:     v.providerType,
		baseURL:          strings.TrimSpace(v.endpointInput.Value()),
		apiKey:           strings.TrimSpace(v.apiKeyInput.Value()),
		discoveryTimeout: discoveryTimeout,
	})
}

func (v *providerPaneView) cancelFetch() {
	if v.fetchCancel == nil {
		return
	}
	v.fetchCancel()
	v.fetchCancel = nil
}

func fetchProviderModelsCmd(request providerFetchRequest) tea.Cmd {
	return func() tea.Msg {
		parent := request.ctx
		if parent == nil {
			parent = context.Background()
		}
		timeout := request.discoveryTimeout
		if timeout <= 0 {
			timeout = runtimepolicy.ModelDiscoveryTimeout
		}
		ctx, cancel := context.WithTimeout(parent, timeout)
		defer cancel()
		models, err := (app.Models{}).Discover(ctx, app.ModelDiscoveryRequest{
			ProviderName: request.providerName,
			ProviderType: request.providerType,
			BaseURL:      request.baseURL,
			APIKey:       request.apiKey,
			Timeout:      timeout,
		})
		return modelsFetchedMsg{
			providerName: request.providerName,
			baseURL:      request.baseURL,
			apiKey:       request.apiKey,
			models:       models,
			requestID:    request.requestID,
			err:          err,
		}
	}
}

type providerSaveRequest struct {
	providerName string
	providerType string
	previousName string
	baseURL      string
	apiKey       string
	defaultModel string
	activate     bool
}

func saveProviderCmd(request providerSaveRequest) tea.Cmd {
	return func() tea.Msg {
		prov := config.ProviderConfig{
			Name:    request.providerName,
			Type:    request.providerType,
			BaseURL: request.baseURL,
			APIKey:  request.apiKey,
		}
		err := (app.Providers{}).Save(app.ProviderSaveRequest{
			Provider:     prov,
			DefaultModel: request.defaultModel,
			PreviousName: request.previousName,
			Activate:     request.activate,
		})
		return providerSavedMsg{
			providerName: request.providerName,
			providerType: request.providerType,
			previousName: request.previousName,
			baseURL:      request.baseURL,
			apiKey:       request.apiKey,
			modelID:      request.defaultModel,
			activated:    request.activate,
			err:          err,
		}
	}
}
