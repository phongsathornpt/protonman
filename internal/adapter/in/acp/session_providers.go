package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

const (
	methodProvidersList   = "protonman/providers/list"
	methodProvidersSave   = "protonman/providers/save"
	methodProvidersDelete = "protonman/providers/delete"
	methodProvidersModels = "protonman/providers/models"
)

// ProviderInfo describes a configured or preset model provider exposed to native clients.
type ProviderInfo struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Protocol     string `json:"protocol"`
	BaseURL      string `json:"baseURL"`
	RequiresKey  bool   `json:"requiresKey"`
	IsConfigured bool   `json:"isConfigured"`
	IsActive     bool   `json:"isActive"`
	IsFree       bool   `json:"isFree"`
	HasKey       bool   `json:"hasKey"`
	DefaultModel string `json:"defaultModel,omitempty"`
}

// ProtonmanProvidersListResult is the payload returned by protonman/providers/list.
type ProtonmanProvidersListResult struct {
	ActiveProvider string         `json:"activeProvider"`
	ActiveModel    string         `json:"activeModel"`
	Providers      []ProviderInfo `json:"providers"`
}

// ProtonmanProvidersSaveParams matches the request for protonman/providers/save.
type ProtonmanProvidersSaveParams struct {
	ProviderName string `json:"providerName"`
	ProviderType string `json:"providerType"`
	PreviousName string `json:"previousName,omitempty"`
	BaseURL      string `json:"baseURL"`
	APIKey       string `json:"apiKey,omitempty"`
	DefaultModel string `json:"defaultModel,omitempty"`
	Activate     bool   `json:"activate"`
}

// ProtonmanProvidersSaveResult is the payload returned on successful save.
type ProtonmanProvidersSaveResult struct {
	Success        bool   `json:"success"`
	ActiveProvider string `json:"activeProvider,omitempty"`
	ActiveModel    string `json:"activeModel,omitempty"`
}

// ProtonmanProvidersDeleteParams matches the request for protonman/providers/delete.
type ProtonmanProvidersDeleteParams struct {
	ProviderName string `json:"providerName"`
}

// ProtonmanProvidersDeleteResult is the payload returned on successful delete.
type ProtonmanProvidersDeleteResult struct {
	Success bool `json:"success"`
}

// ProtonmanProvidersModelsParams matches the request for protonman/providers/models.
type ProtonmanProvidersModelsParams struct {
	ProviderName string `json:"providerName"`
	ProviderType string `json:"providerType,omitempty"`
	BaseURL      string `json:"baseURL,omitempty"`
	APIKey       string `json:"apiKey,omitempty"`
}

// ProtonmanProvidersModelsResult is the payload returned by protonman/providers/models.
type ProtonmanProvidersModelsResult struct {
	ProviderName string               `json:"providerName"`
	Models       []SessionModelOption `json:"models"`
}

// ProvidersControl provides model provider management callbacks to the ACP server.
type ProvidersControl struct {
	List   func(ctx context.Context) (ProtonmanProvidersListResult, error)
	Save   func(ctx context.Context, params ProtonmanProvidersSaveParams) (ProtonmanProvidersSaveResult, error)
	Delete func(ctx context.Context, providerName string) error
	Models func(ctx context.Context, params ProtonmanProvidersModelsParams) ([]SessionModelOption, error)
}

var providersControls sync.Map // map[*Server]ProvidersControl

// WithProvidersControl wires provider management capabilities into the ACP server.
func WithProvidersControl(control ProvidersControl) Option {
	return func(server *Server) {
		providersControls.Store(server, control)
	}
}

func providersControlFor(server *Server) (ProvidersControl, bool) {
	value, ok := providersControls.Load(server)
	if !ok {
		return ProvidersControl{}, false
	}
	control, ok := value.(ProvidersControl)
	return control, ok
}

func (s *Server) dispatchProviders(ctx context.Context, request RPCRequest) (any, bool, error) {
	switch request.Method {
	case methodProvidersList:
		control, ok := providersControlFor(s)
		if !ok || control.List == nil {
			return nil, true, fmt.Errorf("provider management is unavailable")
		}
		res, err := control.List(ctx)
		if err != nil {
			return nil, true, err
		}
		return res, true, nil

	case methodProvidersSave:
		control, ok := providersControlFor(s)
		if !ok || control.Save == nil {
			return nil, true, fmt.Errorf("provider management is unavailable")
		}
		var params ProtonmanProvidersSaveParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			return nil, true, fmt.Errorf("decode %s: %w", methodProvidersSave, err)
		}
		params.ProviderName = strings.TrimSpace(params.ProviderName)
		if params.ProviderName == "" {
			return nil, true, fmt.Errorf("providerName is required")
		}
		res, err := control.Save(ctx, params)
		if err != nil {
			return nil, true, err
		}
		return res, true, nil

	case methodProvidersDelete:
		control, ok := providersControlFor(s)
		if !ok || control.Delete == nil {
			return nil, true, fmt.Errorf("provider management is unavailable")
		}
		var params ProtonmanProvidersDeleteParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			return nil, true, fmt.Errorf("decode %s: %w", methodProvidersDelete, err)
		}
		params.ProviderName = strings.TrimSpace(params.ProviderName)
		if params.ProviderName == "" {
			return nil, true, fmt.Errorf("providerName is required")
		}
		if err := control.Delete(ctx, params.ProviderName); err != nil {
			return nil, true, err
		}
		return ProtonmanProvidersDeleteResult{Success: true}, true, nil

	case methodProvidersModels:
		control, ok := providersControlFor(s)
		if !ok || control.Models == nil {
			return nil, true, fmt.Errorf("provider model discovery is unavailable")
		}
		var params ProtonmanProvidersModelsParams
		if err := json.Unmarshal(request.Params, &params); err != nil {
			return nil, true, fmt.Errorf("decode %s: %w", methodProvidersModels, err)
		}
		params.ProviderName = strings.TrimSpace(params.ProviderName)
		models, err := control.Models(ctx, params)
		if err != nil {
			return nil, true, err
		}
		return ProtonmanProvidersModelsResult{
			ProviderName: params.ProviderName,
			Models:       models,
		}, true, nil

	default:
		return nil, false, nil
	}
}
