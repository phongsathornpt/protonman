package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/projectTHORN/proton/internal/domain/tool"
	"github.com/projectTHORN/proton/internal/sandbox"
)

const maxFetchBytes = 256 * 1024

type webFetchHandler struct {
	policy sandbox.NetworkPolicy
	client *http.Client
}

type webFetchInput struct {
	URL string `json:"url"`
}

// NewWebFetch returns a permission-gated URL fetch adapter.
func NewWebFetch(policy sandbox.NetworkPolicy) tool.Handler {
	if policy.Allowed == nil {
		policy.Allowed = []sandbox.Origin{}
	}
	return webFetchHandler{
		policy: policy,
		client: &http.Client{
			Timeout: 10 * time.Second,
			CheckRedirect: func(req *http.Request, _ []*http.Request) error {
				return policy.AllowURL(req.URL.String())
			},
		},
	}
}

func (webFetchHandler) Definition() tool.Definition {
	return tool.Definition{
		Name:                "web_fetch",
		Description:         "Fetch a URL subject to the sandbox network policy.",
		Kind:                tool.KindWebFetch,
		PermissionDetailKey: "url",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url": map[string]any{
					"type":        "string",
					"description": "HTTP or HTTPS URL to fetch",
				},
			},
			"required": []string{"url"},
		},
	}
}

func (h webFetchHandler) Execute(ctx context.Context, call tool.Call) (tool.Result, error) {
	var input webFetchInput
	if err := json.Unmarshal(call.Arguments, &input); err != nil {
		return tool.Result{}, fmt.Errorf("decode web_fetch arguments: %w", err)
	}
	input.URL = strings.TrimSpace(input.URL)
	if input.URL == "" {
		return tool.Result{}, fmt.Errorf("web_fetch url is required")
	}
	if err := h.policy.AllowURL(input.URL); err != nil {
		return tool.Result{}, err
	}
	if err := ctx.Err(); err != nil {
		return tool.Result{}, fmt.Errorf("before web_fetch: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, input.URL, nil)
	if err != nil {
		return tool.Result{}, fmt.Errorf("build web_fetch request: %w", err)
	}
	response, err := h.client.Do(request)
	if err != nil {
		return tool.Result{}, fmt.Errorf("web_fetch: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxFetchBytes+1))
	if err != nil {
		return tool.Result{}, fmt.Errorf("read web_fetch body: %w", err)
	}
	result := tool.Result{
		CallID:   call.ID,
		ToolName: call.Name,
		Output:   string(body),
	}
	if len(body) > maxFetchBytes {
		result.Output = string(body[:maxFetchBytes])
		result.Truncated = true
	}
	if response.StatusCode >= 400 {
		return result, fmt.Errorf("web_fetch status %d", response.StatusCode)
	}
	return result, nil
}
