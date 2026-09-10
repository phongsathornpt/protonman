package runtime

import (
	"errors"
	"strings"
	"sync/atomic"

	tea "charm.land/bubbletea/v2"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

type asyncOperationID uint64

var asyncOperationSequence atomic.Uint64

var errMissingRuntimeContext = errors.New("tui async operation requires runtime context")
var errStaleConfigMutation = errors.New("stale tui config mutation")

type asyncOperationGate struct {
	latest atomic.Uint64
}

func (g *asyncOperationGate) activate(id asyncOperationID) {
	if g != nil {
		g.latest.Store(uint64(id))
	}
}

func (g *asyncOperationGate) current(id asyncOperationID) bool {
	return g == nil || g.latest.Load() == uint64(id)
}

func (g *asyncOperationGate) invalidate() {
	if g != nil {
		g.latest.Store(0)
	}
}

func nextAsyncOperationID() asyncOperationID {
	return asyncOperationID(asyncOperationSequence.Add(1))
}

func (m *bubbleModel) beginProviderSave(request providerSaveRequest) tea.Cmd {
	id := nextAsyncOperationID()
	m.activeProviderSave = id
	m.configMutationGate.activate(id)
	return saveProviderCmd(id, m.configMutationGate, request)
}

func (m *bubbleModel) beginProviderSelect(providerName string) tea.Cmd {
	id := nextAsyncOperationID()
	m.activeProviderSelect = id
	m.configMutationGate.activate(id)
	reconciledModel := m.reconciledModelForProvider(providerName)
	return saveActiveProviderCmd(id, m.configMutationGate, providerName, reconciledModel)
}

func (m *bubbleModel) reconciledModelForProvider(providerName string) string {
	providerName = strings.TrimSpace(providerName)
	models := m.modelCatalogs.Models(providerName)
	for _, candidate := range models {
		if strings.EqualFold(candidate.ID, m.activeModel) {
			return m.activeModel
		}
	}
	if len(models) > 0 {
		return models[0].ID
	}
	if strings.EqualFold(providerName, model.DefaultOpenCodeName) {
		return model.DefaultOpenCodeModel
	}
	if strings.EqualFold(providerName, m.activeProvider) {
		return m.activeModel
	}
	return ""
}

func (m *bubbleModel) beginProviderDelete(providerName string) tea.Cmd {
	id := nextAsyncOperationID()
	m.activeProviderDelete = id
	m.configMutationGate.activate(id)
	return deleteProviderCmd(id, m.configMutationGate, providerName)
}

func (m *bubbleModel) beginModelSetup(providerName, modelID string, unverified bool) tea.Cmd {
	return m.beginModelSetupSelect(providerName, modelID, m.reasoningPreferenceValue(), unverified)
}

func (m *bubbleModel) beginModelSetupSelect(providerName, modelID string, reasoning sdk.ReasoningEffort, unverified bool) tea.Cmd {
	id := nextAsyncOperationID()
	m.activeModelSetup = id
	m.configMutationGate.activate(id)
	return persistModelSetupCmd(id, m.configMutationGate, providerName, modelID, reasoning, unverified)
}

func (m *bubbleModel) pushProviderPane(view *providerPaneView) {
	m.activeProviderSave = 0
	m.configMutationGate.invalidate()
	m.panes.bottom.push(view)
}
