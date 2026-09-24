package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/phongsathornpt/protonman/internal/base/runtimepolicy"
	"github.com/phongsathornpt/protonman/internal/core/modelcatalog"
)

// RemoteModel is provider-neutral discovered model metadata.
type RemoteModel = modelcatalog.RemoteModel

// ErrUnauthorized indicates that a remote provider rejected credentials (HTTP 401).
var ErrUnauthorized = errors.New("authentication failed (401)")

var openCodeInferenceFreeModels = []RemoteModel{
	{ID: "big-pickle", Name: "Big Pickle", Provider: DefaultOpenCodeName},
	{ID: "mimo-v2.5-free", Name: "MiMo V2.5 Free", Provider: DefaultOpenCodeName},
	{ID: DefaultOpenCodeModel, Name: "Nemotron 3 Super Free", Provider: DefaultOpenCodeName},
}

// OpenCodeInferenceFreeModels returns a copy of the documented keyless chat
// model seed. The Inference API does not expose a public /models contract, so
// discovery must not probe that endpoint and turn a route mismatch into a 404.
func OpenCodeInferenceFreeModels() []RemoteModel {
	models := make([]RemoteModel, len(openCodeInferenceFreeModels))
	copy(models, openCodeInferenceFreeModels)
	return models
}

// IsFreeModel reports whether a given model ID represents an OpenCode free-tier model.
func IsFreeModel(id string) bool {
	idLower := strings.ToLower(strings.TrimSpace(id))
	return strings.HasSuffix(idLower, "-free") || idLower == "big-pickle"
}

// FetchModelsOption configures provider model discovery.
type FetchModelsOption func(*http.Client)

// WithDiscoveryTimeout overrides the model-discovery HTTP timeout.
func WithDiscoveryTimeout(timeout time.Duration) FetchModelsOption {
	return func(client *http.Client) {
		if timeout > 0 {
			client.Timeout = timeout
		}
	}
}

// FetchProviderModels queries an OpenAI-compatible provider model endpoint.
func FetchProviderModels(ctx context.Context, baseURL string, apiKey string, options ...FetchModelsOption) ([]RemoteModel, error) {
	return FetchProviderModelsForProtocol(ctx, ProviderProtocolOpenAI, baseURL, apiKey, options...)
}

// FetchProviderModelsForProtocol queries the model catalog using protocol-specific authentication and response mapping.
func FetchProviderModelsForProtocol(ctx context.Context, protocol ProviderProtocol, baseURL string, apiKey string, options ...FetchModelsOption) ([]RemoteModel, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		if protocol == ProviderProtocolAnthropic {
			baseURL = DefaultAnthropicEndpoint
		} else {
			baseURL = DefaultProtonmanEndpoint
		}
	}

	if protocol == ProviderProtocolOpenAI && IsOpenCodeInferenceEndpoint(baseURL) {
		return OpenCodeInferenceFreeModels(), nil
	}

	client := &http.Client{Timeout: runtimepolicy.ModelDiscoveryTimeout}
	for _, option := range options {
		if option != nil {
			option(client)
		}
	}

	if protocol == ProviderProtocolOpenAI && (IsOpenCodeZenEndpoint(baseURL) || IsOpenCodeGoEndpoint(baseURL)) {
		if strings.TrimSpace(apiKey) == "" {
			return nil, fmt.Errorf("%w: OpenCode Zen/Go requires an API key", ErrUnauthorized)
		}
		models, err := fetchModelsFromURL(ctx, client, baseURL+"/models", apiKey)
		if err != nil {
			return nil, err
		}
		models = filterOpenCodeCatalog(models)
		if len(models) == 0 {
			return nil, errors.New("no supported models returned by OpenCode catalog")
		}
		return models, nil
	}

	if protocol == ProviderProtocolAnthropic {
		return fetchAnthropicModels(ctx, client, baseURL, apiKey)
	}

	// 1. Try standard /models endpoint with Bearer auth (or without auth if apiKey is empty)
	modelsEndpoint := baseURL + "/models"
	models, err := fetchModelsFromURL(ctx, client, modelsEndpoint, apiKey)
	if err == nil && len(models) > 0 {
		return models, nil
	}

	// If 401 Unauthorized, return error directly to let user check their key
	if err != nil && errors.Is(err, ErrUnauthorized) {
		return nil, err
	}

	// 2. Try /public/models fallback (e.g. for protonman public catalog)
	publicEndpoint := baseURL + "/public/models"
	publicModels, publicErr := fetchModelsFromURL(ctx, client, publicEndpoint, "")
	if publicErr == nil && len(publicModels) > 0 {
		return publicModels, nil
	}

	if err != nil {
		return nil, err
	}
	return nil, errors.New("no models returned by endpoint")
}

func fetchModelsFromURL(ctx context.Context, client *http.Client, urlStr string, apiKey string) ([]RemoteModel, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch models: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("%w: invalid or missing API key", ErrUnauthorized)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("endpoint returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return decodeCompatibleModels(body)
}

func fetchAnthropicModels(ctx context.Context, client *http.Client, baseURL string, apiKey string) ([]RemoteModel, error) {
	endpoint := strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(endpoint, "/v1") {
		endpoint += "/models"
	} else {
		endpoint += "/v1/models"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create anthropic models request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")
	if strings.TrimSpace(apiKey) != "" {
		req.Header.Set("x-api-key", apiKey)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch anthropic models: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read anthropic models response: %w", err)
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("%w: invalid or missing API key", ErrUnauthorized)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("endpoint returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var payload struct {
		Data []struct {
			ID             string `json:"id"`
			DisplayName    string `json:"display_name"`
			MaxInputTokens int    `json:"max_input_tokens"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode anthropic models response: %w", err)
	}
	if len(payload.Data) == 0 {
		return nil, errors.New("no models returned by endpoint")
	}
	models := make([]RemoteModel, 0, len(payload.Data))
	for _, item := range payload.Data {
		name := item.DisplayName
		if strings.TrimSpace(name) == "" {
			name = item.ID
		}
		models = append(models, RemoteModel{ID: item.ID, Name: name, MaxInputTokens: item.MaxInputTokens, Provider: DefaultAnthropicName})
	}
	return models, nil
}
