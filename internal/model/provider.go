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
)

const (
	// DefaultProtonmanName is the canonical provider label for protonmanAI.
	DefaultProtonmanName = "protonman"
	// DefaultProtonmanEndpoint is the public API Gateway base URL.
	DefaultProtonmanEndpoint = "https://protonman.dev/api/v1"
)

// RemoteModel describes a model discovered from an OpenAI or protonman endpoint.
type RemoteModel struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	ContextWindow int      `json:"context_window,omitempty"`
	Provider      string   `json:"provider,omitempty"`
	Features      []string `json:"features,omitempty"`
}

// DefaultProtonmanModels provides the standard catalog when offline or fallback.
var DefaultProtonmanModels = []RemoteModel{
	{ID: "deepseek-v4-flash-vision-exp", Name: "DeepSeek V4 Flash Vision (exp)", ContextWindow: 1000000, Provider: "DeepSeek", Features: []string{"json", "tools", "vision"}},
	{ID: "glm-5.3-flash", Name: "GLM-5.3 Flash", ContextWindow: 1048576, Provider: "GLM", Features: []string{"tools", "vision"}},
	{ID: "Qwen3.8-Flash", Name: "Qwen 3.8 Flash", ContextWindow: 1000000, Provider: "Qwen", Features: []string{"text", "vision"}},
	{ID: "muse-spark-1.3-contributor", Name: "Muse Spark 1.3 Contributor", ContextWindow: 1048576, Provider: "Meta", Features: []string{"tools", "vision"}},
	{ID: "MiniMax-M3", Name: "MiniMax M3", ContextWindow: 1000000, Provider: "MiniMax", Features: []string{"text"}},
}

// FetchProviderModels queries a provider's model endpoint to list available models.
func FetchProviderModels(ctx context.Context, baseURL string, apiKey string) ([]RemoteModel, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = DefaultProtonmanEndpoint
	}

	client := &http.Client{Timeout: 10 * time.Second}

	// 1. Try standard /models endpoint with Bearer auth
	modelsEndpoint := baseURL + "/models"
	models, err := fetchModelsFromURL(ctx, client, modelsEndpoint, apiKey)
	if err == nil && len(models) > 0 {
		return models, nil
	}

	// If 401 Unauthorized, return error directly to let user check their key
	if err != nil && strings.Contains(err.Error(), "401") {
		return nil, err
	}

	// 2. Try /public/models fallback (e.g. for protonman public catalog)
	publicEndpoint := baseURL + "/public/models"
	publicModels, publicErr := fetchModelsFromURL(ctx, client, publicEndpoint, "")
	if publicErr == nil && len(publicModels) > 0 {
		return publicModels, nil
	}

	// 3. If baseURL is protonman and remote request failed, return default catalog
	if strings.Contains(baseURL, "protonman.dev") {
		return append([]RemoteModel{}, DefaultProtonmanModels...), nil
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
		return nil, fmt.Errorf("authentication failed (401): invalid or missing API key")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("endpoint returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	// Attempt parsing OpenAI format: {"data": [{"id": "model-id"}]}
	var openAIResp struct {
		Data []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &openAIResp); err == nil && len(openAIResp.Data) > 0 {
		results := make([]RemoteModel, 0, len(openAIResp.Data))
		for _, item := range openAIResp.Data {
			name := item.Name
			if name == "" {
				name = item.ID
			}
			results = append(results, RemoteModel{
				ID:   item.ID,
				Name: name,
			})
		}
		return results, nil
	}

	// Attempt parsing protonman format: {"models": [{"slug": "...", "name": "...", "contextWindow": 1000000}]}
	var protonmanResp struct {
		Models []struct {
			ID            string   `json:"id"`
			Slug          string   `json:"slug"`
			Name          string   `json:"name"`
			ContextWindow int      `json:"contextWindow"`
			Features      []string `json:"features"`
			Provider      struct {
				Name string `json:"name"`
			} `json:"provider"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &protonmanResp); err == nil && len(protonmanResp.Models) > 0 {
		results := make([]RemoteModel, 0, len(protonmanResp.Models))
		for _, item := range protonmanResp.Models {
			id := item.Slug
			if id == "" {
				id = item.ID
			}
			results = append(results, RemoteModel{
				ID:            id,
				Name:          item.Name,
				ContextWindow: item.ContextWindow,
				Provider:      item.Provider.Name,
				Features:      item.Features,
			})
		}
		return results, nil
	}

	return nil, errors.New("unrecognized models response format")
}
