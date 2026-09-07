
type tokenLimitsTestModel struct {
	LanguageModel
	limits TokenLimits
}

func (m tokenLimitsTestModel) TokenLimits() TokenLimits { return m.limits }

func TestModelTokenLimitsUsesRichMetadata(t *testing.T) {
	got := ModelTokenLimits(tokenLimitsTestModel{limits: TokenLimits{ContextWindow: 100, MaxInputTokens: 80, MaxOutputTokens: 20}})
	if got.ContextWindow != 100 || got.MaxInputTokens != 80 || got.MaxOutputTokens != 20 {
		t.Fatalf("ModelTokenLimits() = %+v", got)
	}
}
