package model

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchProviderModelsOpenAIFormat(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/models" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"data":[{"id":"gpt-4o","name":"GPT-4o"},{"id":"o3-mini","name":"o3-mini"}]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	models, err := FetchProviderModels(context.Background(), ts.URL, "test-key")
	if err != nil {
		t.Fatalf("FetchProviderModels() error = %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}
	if models[0].ID != "gpt-4o" || models[1].ID != "o3-mini" {
		t.Fatalf("unexpected models: %+v", models)
	}
}

func TestFetchProviderModelsProtonmanFormat(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/public/models" || r.URL.Path == "/models" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"models": [
					{
						"id": "1",
						"slug": "deepseek-v4-flash-vision-exp",
						"name": "DeepSeek V4 Flash Vision",
						"contextWindow": 1000000,
						"features": ["vision", "tools"],
						"provider": {"name": "DeepSeek"}
					}
				]
			}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	models, err := FetchProviderModels(context.Background(), ts.URL, "any-key")
	if err != nil {
		t.Fatalf("FetchProviderModels() error = %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(models))
	}
	if models[0].ID != "deepseek-v4-flash-vision-exp" || models[0].ContextWindow != 1000000 {
		t.Fatalf("unexpected model: %+v", models[0])
	}
}

func TestFetchProviderModelsUnauthorized(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "invalid key", http.StatusUnauthorized)
	}))
	defer ts.Close()

	_, err := FetchProviderModels(context.Background(), ts.URL, "bad-key")
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("expected 401 error, got: %v", err)
	}
}

func TestFetchProviderModelsFallbackDefaultProtonman(t *testing.T) {
	models, err := FetchProviderModels(context.Background(), "https://invalid-nonexistent.protonman.dev/api/v1", "key")
	if err != nil {
		t.Fatalf("expected fallback default catalog, got error: %v", err)
	}
	if len(models) == 0 {
		t.Fatal("expected default models catalog")
	}
	if models[0].ID != "deepseek-v4-flash-vision-exp" {
		t.Fatalf("unexpected first model: %+v", models[0])
	}
}
