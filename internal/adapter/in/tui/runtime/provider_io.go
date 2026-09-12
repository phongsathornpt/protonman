package runtime

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/runtime/providerio"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/base/runtimepolicy"
)

type modelsFetchedMsg struct {
	providerName string
	baseURL      string
	apiKey       string
	models       []model.RemoteModel
	requestID    asyncOperationID
	err          error
}

type providerSavedMsg struct {
	operationID  asyncOperationID
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
	requestID        asyncOperationID
	providerName     string
	providerType     string
	baseURL          string
	apiKey           string
	discoveryTimeout time.Duration
}

func (v *providerPaneView) beginFetch(parent context.Context, models app.Models, timeouts ...time.Duration) tea.Cmd {
	discoveryTimeout := runtimepolicy.ModelDiscoveryTimeout
	if len(timeouts) > 0 && timeouts[0] > 0 {
		discoveryTimeout = timeouts[0]
	}
	if v.fetchCancel != nil {
		v.fetchCancel()
	}
	if parent == nil {
		v.fetchRequestID = 0
		v.state = providerStateError
		v.errorMessage = errMissingRuntimeContext.Error()
		return nil
	}
	ctx, cancel := context.WithCancel(parent)
	v.fetchCancel = cancel
	v.fetchRequestID = nextAsyncOperationID()
	v.state = providerStateFetching
	return fetchProviderModelsCmd(models, providerFetchRequest{
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

func fetchProviderModelsCmd(models app.Models, request providerFetchRequest) tea.Cmd {
	return func() tea.Msg {
		discovered, err := providerio.Discover(request.ctx, models, providerio.FetchRequest{
			ProviderName: request.providerName,
			ProviderType: request.providerType,
			BaseURL:      request.baseURL,
			APIKey:       request.apiKey,
			Timeout:      request.discoveryTimeout,
		})
		return modelsFetchedMsg{
			providerName: request.providerName,
			baseURL:      request.baseURL,
			apiKey:       request.apiKey,
			models:       discovered,
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

func saveProviderCmd(providers app.Providers, operationID asyncOperationID, gate *asyncOperationGate, request providerSaveRequest) tea.Cmd {
	return func() tea.Msg {
		if !gate.current(operationID) {
			return providerSavedMsg{operationID: operationID, err: errStaleConfigMutation}
		}
		err := providerio.Save(providers, providerio.SaveRequest{
			ProviderName: request.providerName,
			ProviderType: request.providerType,
			PreviousName: request.previousName,
			BaseURL:      request.baseURL,
			APIKey:       request.apiKey,
			DefaultModel: request.defaultModel,
			Activate:     request.activate,
		})
		return providerSavedMsg{
			operationID:  operationID,
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
