package runtime

import (
	"sync/atomic"

	tea "charm.land/bubbletea/v2"
)

type asyncOperationID uint64

var asyncOperationSequence atomic.Uint64

func nextAsyncOperationID() asyncOperationID {
	return asyncOperationID(asyncOperationSequence.Add(1))
}

func (m *bubbleModel) beginProviderSave(request providerSaveRequest) tea.Cmd {
	id := nextAsyncOperationID()
	m.activeProviderSave = id
	return saveProviderCmd(id, request)
}

func (m *bubbleModel) beginProviderSelect(providerName string) tea.Cmd {
	id := nextAsyncOperationID()
	m.activeProviderSelect = id
	return saveActiveProviderCmd(id, providerName)
}

func (m *bubbleModel) beginProviderDelete(providerName string) tea.Cmd {
	id := nextAsyncOperationID()
	m.activeProviderDelete = id
	return deleteProviderCmd(id, providerName)
}

func (m *bubbleModel) beginModelSelect(providerName, modelID string, unverified bool) tea.Cmd {
	id := nextAsyncOperationID()
	m.activeModelSelect = id
	return saveModelSelectionCmd(id, providerName, modelID, unverified)
}

func (m *bubbleModel) pushProviderPane(view *providerPaneView) {
	m.activeProviderSave = 0
	m.bottom.push(view)
}
