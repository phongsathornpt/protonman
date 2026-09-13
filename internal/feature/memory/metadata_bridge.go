package memory

import sdk "github.com/phongsathornpt/protonman/proton-sdk"

var _ sdk.MetadataModel = (*memoryLanguageModel)(nil)

func (m *memoryLanguageModel) Metadata() sdk.ModelMetadata {
	return sdk.ModelMetadataOf(m.base)
}
