package usecase_test

import (
	"testing"

	"github.com/phongsathornpt/protonman/proton-sdk/domain"
	"github.com/phongsathornpt/protonman/proton-sdk/usecase"
)

type metadataTestModel struct {
	mockModel
	metadata domain.ModelMetadata
}

func (m *metadataTestModel) Metadata() domain.ModelMetadata {
	return m.metadata
}

type legacyLimitsModel struct {
	mockModel
	limits domain.TokenLimits
}

func (m *legacyLimitsModel) TokenLimits() domain.TokenLimits {
	return m.limits
}

type legacyContextModel struct {
	mockModel
	window int
}

func (m *legacyContextModel) ContextWindow() int {
	return m.window
}

func TestModelMetadataOf(t *testing.T) {
	metaModel := &metadataTestModel{
		metadata: domain.ModelMetadata{
			TokenLimits: domain.TokenLimits{ContextWindow: 8192, MaxInputTokens: 4096, MaxOutputTokens: 2048},
		},
	}
	meta := usecase.ModelMetadataOf(metaModel)
	if meta.TokenLimits.ContextWindow != 8192 {
		t.Fatalf("expected context window 8192, got %d", meta.TokenLimits.ContextWindow)
	}

	limitsModel := &legacyLimitsModel{
		limits: domain.TokenLimits{ContextWindow: 4096},
	}
	limits := usecase.ModelTokenLimits(limitsModel)
	if limits.ContextWindow != 4096 {
		t.Fatalf("expected 4096, got %d", limits.ContextWindow)
	}

	contextModel := &legacyContextModel{
		window: 2048,
	}
	window := usecase.ModelContextWindow(contextModel)
	if window != 2048 {
		t.Fatalf("expected 2048, got %d", window)
	}

	if (usecase.ModelMetadataOf(nil) != domain.ModelMetadata{}) {
		t.Fatal("nil model should return zero ModelMetadata")
	}
}
