package modelprofile

import sdk "github.com/projectTHORN/proton/proton-sdk"

func mergeProfile(dst *Resolved, src Profile) {
	if src.Name != "" {
		dst.ProfileName = src.Name
	}
	mergeSupportWithSource(&dst.Capabilities.Tools, src.Capabilities.Tools, &dst.Provenance.Tools, MetadataSourceBuiltin)
	mergeSupportWithSource(&dst.Capabilities.Vision, src.Capabilities.Vision, &dst.Provenance.Vision, MetadataSourceBuiltin)
	mergeSupportWithSource(&dst.Capabilities.Reasoning, src.Capabilities.Reasoning, &dst.Provenance.ReasoningSupport, MetadataSourceBuiltin)
	mergeSupportWithSource(&dst.Capabilities.ToolChoiceRequired, src.Capabilities.ToolChoiceRequired, &dst.Provenance.ToolChoiceRequired, MetadataSourceBuiltin)
	mergeSupportWithSource(&dst.Reasoning.Support, src.Reasoning.Support, &dst.Provenance.ReasoningSupport, MetadataSourceBuiltin)
	if len(src.Reasoning.Levels) > 0 {
		dst.Reasoning.Levels = append([]sdk.ReasoningEffort(nil), src.Reasoning.Levels...)
		dst.Provenance.ReasoningLevels = MetadataSourceBuiltin
	}
	if src.Reasoning.Default != sdk.ReasoningDefault {
		dst.Reasoning.Default = src.Reasoning.Default
		dst.Provenance.ReasoningDefault = MetadataSourceBuiltin
	}
	mergeSupport(&dst.Sampling.Temperature, src.Sampling.Temperature)
	mergeSupport(&dst.Sampling.TopP, src.Sampling.TopP)
	mergeSupport(&dst.Sampling.TopK, src.Sampling.TopK)
	if src.ContextWindow > 0 {
		dst.ContextWindow = src.ContextWindow
		dst.Provenance.ContextWindow = MetadataSourceBuiltin
	}
	if src.MaxInputTokens > 0 {
		dst.MaxInputTokens = src.MaxInputTokens
		dst.Provenance.MaxInputTokens = MetadataSourceBuiltin
	}
	if src.MaxOutputTokens > 0 {
		dst.MaxOutputTokens = src.MaxOutputTokens
		dst.Provenance.MaxOutputTokens = MetadataSourceBuiltin
	}
	if len(src.PromptHints) > 0 {
		dst.PromptHints = append([]string(nil), src.PromptHints...)
		dst.Provenance.PromptHints = MetadataSourceBuiltin
	}
	if src.ToolSchemaDialect != ToolSchemaDefault {
		dst.ToolSchemaDialect = src.ToolSchemaDialect
		dst.Provenance.ToolSchemaDialect = MetadataSourceBuiltin
	}
}

func mergeCatalog(dst *Resolved, src CatalogMetadata) {
	if src.Tools != nil {
		dst.Capabilities.Tools = supportFromPointer(src.Tools)
		dst.Provenance.Tools = MetadataSourceCatalog
	}
	if src.Vision != nil {
		dst.Capabilities.Vision = supportFromPointer(src.Vision)
		dst.Provenance.Vision = MetadataSourceCatalog
	}
	if src.ToolChoiceRequired != nil {
		dst.Capabilities.ToolChoiceRequired = supportFromPointer(src.ToolChoiceRequired)
		dst.Provenance.ToolChoiceRequired = MetadataSourceCatalog
	}
	if src.ContextWindow > 0 {
		dst.ContextWindow = src.ContextWindow
		dst.Provenance.ContextWindow = MetadataSourceCatalog
	}
	if src.MaxInputTokens > 0 {
		dst.MaxInputTokens = src.MaxInputTokens
		dst.Provenance.MaxInputTokens = MetadataSourceCatalog
	}
	if src.MaxOutputTokens > 0 {
		dst.MaxOutputTokens = src.MaxOutputTokens
		dst.Provenance.MaxOutputTokens = MetadataSourceCatalog
	}
	if src.Reasoning == nil {
		return
	}
	if src.Reasoning.Supported != nil {
		support := supportFromPointer(src.Reasoning.Supported)
		dst.Capabilities.Reasoning = support
		dst.Reasoning.Support = support
		dst.Provenance.ReasoningSupport = MetadataSourceCatalog
		if support == SupportNo {
			dst.Reasoning.Levels = nil
			dst.Reasoning.Default = sdk.ReasoningDefault
			return
		}
	}
	if len(src.Reasoning.Levels) > 0 {
		dst.Reasoning.Levels = append([]sdk.ReasoningEffort(nil), src.Reasoning.Levels...)
		dst.Provenance.ReasoningLevels = MetadataSourceCatalog
	}
	if src.Reasoning.Default != sdk.ReasoningDefault {
		dst.Reasoning.Default = src.Reasoning.Default
		dst.Provenance.ReasoningDefault = MetadataSourceCatalog
	}
}

func mergeSupport(dst *Support, src Support) {
	if src != SupportUnknown {
		*dst = src
	}
}

func mergeSupportWithSource(dst *Support, src Support, provenance *MetadataSource, source MetadataSource) {
	if src == SupportUnknown {
		return
	}
	*dst = src
	*provenance = source
}
