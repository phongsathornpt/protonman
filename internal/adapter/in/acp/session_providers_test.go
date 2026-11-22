package acp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/phongsathornpt/protonman/internal/core/permission"
)

func TestDispatchProvidersList(t *testing.T) {
	ctx := context.Background()
	server := newTestServer(t, permission.ModeAsk)
	WithProvidersControl(ProvidersControl{
		List: func(ctx context.Context) (ProtonmanProvidersListResult, error) {
			return ProtonmanProvidersListResult{
				ActiveProvider: "protonman",
				ActiveModel:    "claude-3-7-sonnet",
				Providers: []ProviderInfo{
					{
						ID:           "protonman",
						Name:         "Protonman",
						Protocol:     "openai",
						BaseURL:      "https://protonman.dev/api/v1",
						RequiresKey:  true,
						IsConfigured: true,
						IsActive:     true,
						HasKey:       true,
						DefaultModel: "claude-3-7-sonnet",
					},
					{
						ID:          "ollama",
						Name:        "Ollama (Local)",
						Protocol:    "openai",
						BaseURL:     "http://localhost:11434/v1",
						RequiresKey: false,
						IsFree:      true,
					},
				},
			}, nil
		},
	})(server)

	res, handled, err := server.dispatchProviders(ctx, RPCRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`"1"`),
		Method:  methodProvidersList,
	})
	if err != nil || !handled {
		t.Fatalf("dispatchProviders list error = %v, handled = %v", err, handled)
	}
	result, ok := res.(ProtonmanProvidersListResult)
	if !ok {
		t.Fatalf("expected ProtonmanProvidersListResult, got %T", res)
	}
	if result.ActiveProvider != "protonman" {
		t.Errorf("expected activeProvider 'protonman', got %q", result.ActiveProvider)
	}
	if len(result.Providers) != 2 {
		t.Errorf("expected 2 providers, got %d", len(result.Providers))
	}
}

func TestDispatchProvidersSave(t *testing.T) {
	ctx := context.Background()
	server := newTestServer(t, permission.ModeAsk)
	var savedParams ProtonmanProvidersSaveParams
	WithProvidersControl(ProvidersControl{
		Save: func(ctx context.Context, params ProtonmanProvidersSaveParams) (ProtonmanProvidersSaveResult, error) {
			savedParams = params
			return ProtonmanProvidersSaveResult{
				Success:        true,
				ActiveProvider: params.ProviderName,
				ActiveModel:    params.DefaultModel,
			}, nil
		},
	})(server)

	body, _ := json.Marshal(ProtonmanProvidersSaveParams{
		ProviderName: "openai",
		ProviderType: "openai",
		BaseURL:      "https://api.openai.com/v1",
		APIKey:       "sk-test",
		DefaultModel: "gpt-4o",
		Activate:     true,
	})
	res, handled, err := server.dispatchProviders(ctx, RPCRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`"2"`),
		Method:  methodProvidersSave,
		Params:  body,
	})
	if err != nil || !handled {
		t.Fatalf("dispatchProviders save error = %v, handled = %v", err, handled)
	}
	result, ok := res.(ProtonmanProvidersSaveResult)
	if !ok || !result.Success {
		t.Fatalf("expected success save result, got %#v", res)
	}
	if savedParams.ProviderName != "openai" || savedParams.APIKey != "sk-test" || !savedParams.Activate {
		t.Fatalf("unexpected saved params: %#v", savedParams)
	}

	// Test missing providerName error
	badBody, _ := json.Marshal(ProtonmanProvidersSaveParams{
		ProviderName: "",
	})
	_, _, err = server.dispatchProviders(ctx, RPCRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`"3"`),
		Method:  methodProvidersSave,
		Params:  badBody,
	})
	if err == nil {
		t.Fatal("expected error for empty providerName, got nil")
	}
}

func TestDispatchProvidersDelete(t *testing.T) {
	ctx := context.Background()
	server := newTestServer(t, permission.ModeAsk)
	deletedName := ""
	WithProvidersControl(ProvidersControl{
		Delete: func(ctx context.Context, providerName string) error {
			deletedName = providerName
			return nil
		},
	})(server)

	body, _ := json.Marshal(ProtonmanProvidersDeleteParams{
		ProviderName: "my-custom",
	})
	res, handled, err := server.dispatchProviders(ctx, RPCRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`"4"`),
		Method:  methodProvidersDelete,
		Params:  body,
	})
	if err != nil || !handled {
		t.Fatalf("dispatchProviders delete error = %v, handled = %v", err, handled)
	}
	if deletedName != "my-custom" {
		t.Fatalf("expected delete for 'my-custom', got %q", deletedName)
	}
	result, ok := res.(ProtonmanProvidersDeleteResult)
	if !ok || !result.Success {
		t.Fatalf("expected success delete result, got %#v", res)
	}
}

func TestDispatchProvidersModels(t *testing.T) {
	ctx := context.Background()
	server := newTestServer(t, permission.ModeAsk)
	WithProvidersControl(ProvidersControl{
		Models: func(ctx context.Context, params ProtonmanProvidersModelsParams) ([]SessionModelOption, error) {
			if params.ProviderName == "ollama" {
				return []SessionModelOption{
					{ID: "llama3.3:70b", Name: "Llama 3.3 70B"},
				}, nil
			}
			return nil, errors.New("provider not found")
		},
	})(server)

	body, _ := json.Marshal(ProtonmanProvidersModelsParams{
		ProviderName: "ollama",
	})
	res, handled, err := server.dispatchProviders(ctx, RPCRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`"5"`),
		Method:  methodProvidersModels,
		Params:  body,
	})
	if err != nil || !handled {
		t.Fatalf("dispatchProviders models error = %v, handled = %v", err, handled)
	}
	result, ok := res.(ProtonmanProvidersModelsResult)
	if !ok || len(result.Models) != 1 || result.Models[0].ID != "llama3.3:70b" {
		t.Fatalf("unexpected models result: %#v", res)
	}
}
