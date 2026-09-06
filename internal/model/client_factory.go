package model

// NewProviderClient creates the model client for a configured provider.
//
// The official SDK is intentionally selected only for the explicit OpenAI
// provider or the api.openai.com endpoint. Other compatible providers retain
// the existing transport until their protocol extensions are validated.
func NewProviderClient(
	providerName string,
	baseURL string,
	apiKey string,
	modelID string,
	opts ...OpenAIOption,
) Client {
	if isOfficialOpenAIProvider(providerName, baseURL) {
		return NewOfficialOpenAIClient(baseURL, apiKey, modelID, opts...)
	}
	return NewOpenAIClient(baseURL, apiKey, modelID, opts...)
}

func isOfficialOpenAIProvider(providerName string, baseURL string) bool {
	return IsProvider(DefaultOpenAIName, providerName, baseURL)
}
