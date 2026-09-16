package memory

import (
	"github.com/phongsathornpt/protonman/proton-sdk/domain"
	"github.com/phongsathornpt/protonman/proton-sdk/port"
	"github.com/phongsathornpt/protonman/proton-sdk/usecase"
)

var _ port.MetadataModel = (*memoryLanguageModel)(nil)

func (m *memoryLanguageModel) Metadata() domain.ModelMetadata {
	return usecase.ModelMetadataOf(m.base)
}
