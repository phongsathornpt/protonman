package webtool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/phongsathornpt/protonman/internal/base/runtimepolicy"

	"github.com/phongsathornpt/protonman/internal/base/buildinfo"
	"github.com/phongsathornpt/protonman/internal/core/tool"
	"github.com/phongsathornpt/protonman/internal/platform/sandbox"
)

const maxFetchBytes = 256 * 1024
const defaultWebSearchEndpoint = "https://html.duckduckgo.com/html/"

var webSearchResultRE = regexp.MustCompile(`(?is)<a[^>]+class="result__a"[^>]+href="([^"]+)"[^>]*>(.*?)</a>`)
var webSearchTagRE = regexp.MustCompile(`(?s)<[^>]+>`)

type WebFetchOption func(*webFetchHandler)

func WithWebSearchEndpoint(endpoint string) WebFetchOption {
	return func(handler *webFetchHandler) {
		if strings.TrimSpace(endpoint) != "" {
			handler.searchEndpoint = strings.TrimSpace(endpoint)
		}
	}
}

func WithWebFetchTimeout(timeout time.Duration) WebFetchOption {
	return func(handler *webFetchHandler) {
		if timeout > 0 && handler.client != nil {
			handler.client.Timeout = timeout
		}
	}
}

type webFetchHandler struct {
	policy         sandbox.NetworkPolicy
	client         *http.Client
	searchEndpoint string
}

type ipResolver interface {
	LookupIP(context.Context, string, string) ([]net.IP, error)
}

type contextDialer interface {
	DialContext(context.Context, string, string) (net.Conn, error)
}

type webFetchInput struct {
	Action string `json:"action"`
	URL    string `json:"url"`
	Query  string `json:"query"`
	Limit  int    `json:"limit"`
}

// NewWebFetch returns a permission-gated URL fetch adapter.
func NewWebFetch(policy sandbox.NetworkPolicy, options ...WebFetchOption) tool.Handler {
	if policy.Allowed == nil {
		policy.Allowed = []sandbox.Origin{}
	}
	handler := webFetchHandler{
		policy:         policy,
		searchEndpoint: defaultWebSearchEndpoint,
		client: &http.Client{
			Timeout:   runtimepolicy.WebFetchTimeout,
			Transport: newWebFetchTransport(policy, net.DefaultResolver, &net.Dialer{}),
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

func newWebFetchTransport(policy sandbox.NetworkPolicy, resolver ipResolver, dialer contextDialer) *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, fmt.Errorf("split web dial address %q: %w", address, err)
		}
		ips := []net.IP(nil)
		if literal := net.ParseIP(host); literal != nil {
			ips = []net.IP{literal}
		} else {
			ips, err = resolver.LookupIP(ctx, "ip", host)
			if err != nil {
				return nil, fmt.Errorf("resolve web host %q: %w", host, err)
			}
		}
		if err := policy.ValidateResolvedAddresses(ips); err != nil {
			return nil, err
		}
		var lastErr error
		for _, ip := range ips {
			conn, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if dialErr == nil {
				return conn, nil
			}
			lastErr = dialErr
		}
		return nil, fmt.Errorf("dial web destination %q: %w", address, lastErr)
	}
	return transport
}

func (webFetchHandler) Definition() tool.Definition {
	return tool.Definition{
		Name:        tool.NameWeb,
		Description: "Web capability. Use action=search to discover sources, or action=fetch when the target HTTP or HTTPS URL is already known.",
		Kind:        tool.KindForName("web"),
		Mutability:  tool.MutabilityReadOnly,
		Safety:      tool.SafetyContract{MutationDomain: tool.MutationDomainNone, MutationSafety: tool.MutationSafetyNone, CheckpointPolicy: tool.CheckpointPolicyNone, Boundary: tool.BoundaryPolicyExternalRead},
		Evidence:    tool.EvidenceExternal,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]any{"type": "string", "enum": []string{"fetch", "search"}, "description": "Web operation to perform"},
				"url":    map[string]any{"type": "string", "description": "HTTP or HTTPS URL for action=fetch"},
				"query":  map[string]any{"type": "string", "description": "Search query for action=search"},
				"limit":  map[string]any{"type": "integer", "minimum": 1, "maximum": 10, "description": "Maximum search results for action=search; defaults to 5"},
			},
			"required":             []string{"action"},
			"additionalProperties": false,
		},
	}
}

func (h webFetchHandler) Execute(ctx context.Context, call tool.Call) (tool.Result, error) {
	var input webFetchInput
	if err := json.Unmarshal(call.Arguments, &input); err != nil {
		return tool.Result{}, tool.WrapToolError(tool.ErrorCodeInvalidArguments, "decode web arguments", err)
	}
	input.Action = strings.ToLower(strings.TrimSpace(input.Action))
	if input.Action == "" {
		input.Action = "fetch"
	}
	if input.Action == "search" {
		return h.search(ctx, call, input)
	}
	if input.Action != "fetch" {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, "web action must be fetch or search")
	}
	input.URL = strings.TrimSpace(input.URL)
	if input.URL == "" {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, "web url is required")
	}

	parsedURL, err := url.Parse(input.URL)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.Hostname() == "" {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, "web url must be a valid http or https URL")
	}

	if err := h.policy.AllowURL(input.URL); err != nil {
		return tool.Result{}, err
	}
	if err := ctx.Err(); err != nil {
		return tool.Result{}, fmt.Errorf("before web: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, input.URL, nil)
	if err != nil {
		return tool.Result{}, fmt.Errorf("build web request: %w", err)
	}
	request.Header.Set("User-Agent", buildinfo.WebUserAgent())
	request.Header.Set("Accept", "text/html,application/xhtml+xml,application/json,text/plain;q=0.9,*/*;q=0.8")

	response, err := h.client.Do(request)
	if err != nil {
		return tool.Result{}, fmt.Errorf("web: %w", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(io.LimitReader(response.Body, maxFetchBytes+1))
	if err != nil {
		return tool.Result{}, fmt.Errorf("read web body: %w", err)
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
			return result, fmt.Errorf("web status %d: %s", response.StatusCode, statusText)
		}
		return result, fmt.Errorf("web status %d", response.StatusCode)
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

func (h webFetchHandler) PermissionDetail(arguments json.RawMessage) string {
	var input webFetchInput
	if json.Unmarshal(arguments, &input) != nil {
		return ""
	}
	if strings.EqualFold(strings.TrimSpace(input.Action), "search") {
		return strings.TrimSpace(input.Query)
	}
	return strings.TrimSpace(input.URL)
}

type webSearchResult struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

func (h webFetchHandler) search(ctx context.Context, call tool.Call, input webFetchInput) (tool.Result, error) {
	query := strings.TrimSpace(input.Query)
	if query == "" {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, "web query is required for action=search")
	}
	limit := input.Limit
	if limit == 0 {
		limit = 5
	}
	if limit < 1 || limit > 10 {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeInvalidArguments, "web search limit must be between 1 and 10")
	}
	endpoint, err := url.Parse(h.searchEndpoint)
	if err != nil || endpoint.Scheme == "" || endpoint.Hostname() == "" {
		return tool.Result{}, tool.NewToolError(tool.ErrorCodeExecution, "web search endpoint is invalid")
	}
	values := endpoint.Query()
	values.Set("q", query)
	endpoint.RawQuery = values.Encode()
	if err := h.policy.AllowURL(endpoint.String()); err != nil {
		return tool.Result{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return tool.Result{}, fmt.Errorf("build web search request: %w", err)
	}
	request.Header.Set("User-Agent", buildinfo.WebUserAgent())
	request.Header.Set("Accept", "text/html,application/xhtml+xml")
	response, err := h.client.Do(request)
	if err != nil {
		return tool.Result{}, fmt.Errorf("web search: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxFetchBytes+1))
	if err != nil {
		return tool.Result{}, fmt.Errorf("read web search body: %w", err)
	}
	if response.StatusCode >= 400 {
		return tool.Result{}, fmt.Errorf("web search status %d", response.StatusCode)
	}
	results := parseWebSearchResults(body, limit)
	if len(results) == 0 {
		return tool.Result{CallID: call.ID, ToolName: call.Name, Output: "no web search results"}, nil
	}
	var out strings.Builder
	for i, result := range results {
		fmt.Fprintf(&out, "%d. %s\n   %s", i+1, result.Title, result.URL)
		if i+1 < len(results) {
			out.WriteByte('\n')
		}
	}
	structured, _ := json.Marshal(map[string]any{"query": query, "results": results})
	return tool.Result{CallID: call.ID, ToolName: call.Name, Output: out.String(), StructuredOutput: structured}, nil
}

func parseWebSearchResults(body []byte, limit int) []webSearchResult {
	matches := webSearchResultRE.FindAllSubmatch(body, limit)
	results := make([]webSearchResult, 0, len(matches))
	for _, match := range matches {
		if len(match) < 3 {
			continue
		}
		title := strings.TrimSpace(html.UnescapeString(webSearchTagRE.ReplaceAllString(string(match[2]), "")))
		rawURL := html.UnescapeString(string(match[1]))
		resultURL := normalizeSearchResultURL(rawURL)
		if title == "" || resultURL == "" {
			continue
		}
		results = append(results, webSearchResult{Title: title, URL: resultURL})
	}
	return results
}

func normalizeSearchResultURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	if encoded := parsed.Query().Get("uddg"); encoded != "" {
		decoded, err := url.QueryUnescape(encoded)
		if err == nil {
			parsed, err = url.Parse(decoded)
			if err != nil {
				return ""
			}
		}
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ""
	}
	return parsed.String()
}
