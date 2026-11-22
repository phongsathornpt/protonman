package memory

import (
	"github.com/phongsathornpt/protonman/pkg/proton-sdk/domain"
	"github.com/phongsathornpt/protonman/pkg/proton-sdk/port"
	"github.com/phongsathornpt/protonman/pkg/proton-sdk/usecase"
)

var _ port.MetadataModel = (*memoryLanguageModel)(nil)

func (m *memoryLanguageModel) Metadata() domain.ModelMetadata {
	return usecase.ModelMetadataOf(m.base)
}
