package builtin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/projectTHORN/proton/internal/runtimepolicy"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/projectTHORN/proton/internal/buildinfo"
	"github.com/projectTHORN/proton/internal/sandbox"
	"github.com/projectTHORN/proton/internal/tool"
)

const maxFetchBytes = 256 * 1024

type WebFetchOption func(*webFetchHandler)

func WithWebFetchTimeout(timeout time.Duration) WebFetchOption {
	return func(handler *webFetchHandler) {
		if timeout > 0 && handler.client != nil {
			handler.client.Timeout = timeout
		}
	}
}

type webFetchHandler struct {
	policy sandbox.NetworkPolicy
	client *http.Client
}

type webFetchInput struct {
	URL string `json:"url"`
}

// NewWebFetch returns a permission-gated URL fetch adapter.
func NewWebFetch(policy sandbox.NetworkPolicy, options ...WebFetchOption) tool.Handler {
	if policy.Allowed == nil {
		policy.Allowed = []sandbox.Origin{}
	}
	handler := webFetchHandler{
		policy: policy,
		client: &http.Client{
			Timeout: runtimepolicy.WebFetchTimeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 10 {
					return fmt.Errorf("stopped after 10 redirects")
				}
				if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
					return fmt.Errorf("redirect to unsupported scheme %q", req.URL.Scheme)
				}
				return policy.AllowURL(req.URL.String())
			},
		},
	}
	for _, option := range options {
		if option != nil {
			option(&handler)
		}
	}
	return handler
}

func (webFetchHandler) Definition() tool.Definition {
	return tool.Definition{
		Name:                "web_fetch",
		Description:         "Fetch a URL subject to the sandbox network policy.",
		Kind:                tool.KindWebFetch,
		Mutability:          tool.MutabilityReadOnly,
		Safety:              tool.SafetyContract{MutationDomain: tool.MutationDomainNone, MutationSafety: tool.MutationSafetyNone, CheckpointPolicy: tool.CheckpointPolicyNone, Boundary: tool.BoundaryPolicyExternalRead},
		PermissionDetailKey: "url",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url": map[string]any{
					"type":        "string",
					"description": "HTTP or HTTPS URL to fetch",
				},
			},
			"required":             []string{"url"},
			"additionalProperties": false,
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

	parsedURL, err := url.Parse(input.URL)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.Hostname() == "" {
		return tool.Result{}, fmt.Errorf("web_fetch url must be a valid http or https URL")
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
	request.Header.Set("User-Agent", buildinfo.WebUserAgent())
	request.Header.Set("Accept", "text/html,application/xhtml+xml,application/json,text/plain;q=0.9,*/*;q=0.8")

	response, err := h.client.Do(request)
	if err != nil {
		return tool.Result{}, fmt.Errorf("web_fetch: %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, maxFetchBytes+1))
	if err != nil {
		return tool.Result{}, fmt.Errorf("read web_fetch body: %w", err)
	}

	contentType := response.Header.Get("Content-Type")
	if isBinaryContent(contentType, body) {
		displayType := contentType
		if displayType == "" {
			displayType = http.DetectContentType(body)
		}
		result := tool.Result{
			CallID:   call.ID,
			ToolName: call.Name,
			Output:   fmt.Sprintf("[binary content omitted: %s, %d bytes]", displayType, len(body)),
		}
		return result, nil
	}

	output := string(body)
	var truncated bool
	if len(body) > maxFetchBytes {
		output = string(body[:maxFetchBytes]) + "\n[output truncated at 256 KiB]"
		truncated = true
	}

	result := tool.Result{
		CallID:    call.ID,
		ToolName:  call.Name,
		Output:    output,
		Truncated: truncated,
	}
	if response.StatusCode >= 400 {
		statusText := http.StatusText(response.StatusCode)
		if statusText != "" {
			return result, fmt.Errorf("web_fetch status %d: %s", response.StatusCode, statusText)
		}
		return result, fmt.Errorf("web_fetch status %d", response.StatusCode)
	}
	return result, nil
}

func isBinaryContent(contentType string, body []byte) bool {
	mediaType := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	if strings.HasPrefix(mediaType, "image/") ||
		strings.HasPrefix(mediaType, "audio/") ||
		strings.HasPrefix(mediaType, "video/") ||
		mediaType == "application/octet-stream" ||
		mediaType == "application/zip" ||
		mediaType == "application/pdf" ||
		mediaType == "application/gzip" {
		return true
	}
	checkLen := min(len(body), 512)
	return bytes.IndexByte(body[:checkLen], 0) != -1
}
