package model

// NewProviderClient creates the model client for a configured provider.
// OpenAI-compatible providers are routed through proton-sdk while the legacy
// client constructors remain available during the migration.
func NewProviderClient(
	providerName string,
	baseURL string,
	apiKey string,
	modelID string,
	opts ...OpenAIOption,
) Client {
	return newSDKOpenAIClient(providerName, baseURL, apiKey, modelID, opts...)
}
