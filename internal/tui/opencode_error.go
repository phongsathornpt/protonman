package tui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/projectTHORN/proton/internal/appdirs"
	applicationturn "github.com/projectTHORN/proton/internal/turn"
)

// OpenCodeErrorKind classifies errors matching the OpenCode error taxonomy.
type OpenCodeErrorKind string

const (
	ErrorKindModelNotFound    OpenCodeErrorKind = "model_not_found"
	ErrorKindContextOverflow  OpenCodeErrorKind = "context_overflow"
	ErrorKindAuthentication   OpenCodeErrorKind = "authentication"
	ErrorKindForbidden        OpenCodeErrorKind = "forbidden"
	ErrorKindRateLimit        OpenCodeErrorKind = "rate_limit"
	ErrorKindQuotaExceeded    OpenCodeErrorKind = "quota_exceeded"
	ErrorKindServerOverloaded OpenCodeErrorKind = "server_overloaded"
	ErrorKindStreamTimeout    OpenCodeErrorKind = "stream_timeout"
	ErrorKindInvalidPrompt    OpenCodeErrorKind = "invalid_prompt"
	ErrorKindMCPFailed        OpenCodeErrorKind = "mcp_failed"
	ErrorKindConfigInvalid    OpenCodeErrorKind = "config_invalid"
	ErrorKindConfigTypo       OpenCodeErrorKind = "config_typo"
	ErrorKindToolFailed       OpenCodeErrorKind = "tool_failed"
	ErrorKindToolDispatch     OpenCodeErrorKind = "tool_dispatch"
	ErrorKindMaxRounds        OpenCodeErrorKind = "max_rounds"
	ErrorKindPermissionDenied OpenCodeErrorKind = "permission_denied"
	ErrorKindCancelled        OpenCodeErrorKind = "cancelled"
	ErrorKindGeneric          OpenCodeErrorKind = "generic"
)

// ClassifiedError represents a structured, user-friendly error with actionable guidance.
type ClassifiedError struct {
	Kind        OpenCodeErrorKind
	Title       string
	Badge       string
	Message     string
	Suggestions []string
	RawDetails  string
	Code        string
	Retryable   bool
}

var overflowPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)prompt is too long`),
	regexp.MustCompile(`(?i)request_too_large`),
	regexp.MustCompile(`(?i)input is too long for requested model`),
	regexp.MustCompile(`(?i)exceeds the context window`),
	regexp.MustCompile(`(?i)exceeds (?:the )?(?:model'?s )?maximum context length(?: of [\d,]+ tokens?|\s*\([\d,]+\))`),
	regexp.MustCompile(`(?i)input token count.*exceeds the maximum`),
	regexp.MustCompile(`(?i)tokens in request more than max tokens allowed`),
	regexp.MustCompile(`(?i)maximum prompt length is \d+`),
	regexp.MustCompile(`(?i)reduce the length of the messages`),
	regexp.MustCompile(`(?i)maximum context length is \d+ tokens`),
	regexp.MustCompile(`(?i)exceeds (?:the )?maximum allowed input length of [\d,]+ tokens?`),
	regexp.MustCompile(`(?i)input \(\d+ tokens\) is longer than the model'?s context length \(\d+ tokens\)`),
	regexp.MustCompile(`(?i)exceeds the limit of \d+`),
	regexp.MustCompile(`(?i)exceeds the available context size`),
	regexp.MustCompile(`(?i)greater than the context length`),
	regexp.MustCompile(`(?i)context window exceeds limit`),
	regexp.MustCompile(`(?i)exceeded model token limit`),
	regexp.MustCompile(`(?i)context[_ ]length[_ ]exceeded`),
	regexp.MustCompile(`(?i)request entity too large`),
	regexp.MustCompile(`(?i)context length is only \d+ tokens`),
	regexp.MustCompile(`(?i)input length.*exceeds.*context length`),
	regexp.MustCompile(`(?i)prompt too long; exceeded (?:max )?context length`),
	regexp.MustCompile(`(?i)too large for model with \d+ maximum context length`),
	regexp.MustCompile(`(?i)prompt has [\d,]+ tokens?, but the configured context size is [\d,]+ tokens?`),
	regexp.MustCompile(`(?i)model_context_window_exceeded`),
	regexp.MustCompile(`(?i)too many tokens`),
	regexp.MustCompile(`(?i)token limit exceeded`),
}

var overflowExclusions = []*regexp.Regexp{
	regexp.MustCompile(`(?i)^(throttling error|service unavailable):`),
	regexp.MustCompile(`(?i)rate limit`),
	regexp.MustCompile(`(?i)too many requests`),
}

var statusPattern = regexp.MustCompile(`(?i)(?:status|code)\s*[:=]?\s*(\d{3})`)
var modelErrorPattern = regexp.MustCompile(`(?i)model\s+([a-zA-Z0-9_.:/-]+)\s+is not supported`)
var modelNotFoundPattern = regexp.MustCompile(`(?i)model\s+['"]?([a-zA-Z0-9_.:/-]+)['"]?\s+(?:not found|does not exist)`)

// parsedBody stores extracted fields from an API error payload.
type parsedBody struct {
	ErrorType string
	ErrorCode string
	Message   string
}

func parseAPIErrorPayload(body string) (parsedBody, bool) {
	trimmed := strings.TrimSpace(body)
	if !strings.HasPrefix(trimmed, "{") {
		return parsedBody{}, false
	}

	// 1. Try OpenCode Zen schema: {"type":"error","error":{"type":"ModelError","message":"..."}}
	var openCodeResp struct {
		Type  string `json:"type"`
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
			Code    string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(trimmed), &openCodeResp); err == nil && (openCodeResp.Type == "error" || openCodeResp.Error.Type != "" || openCodeResp.Error.Message != "") {
		return parsedBody{
			ErrorType: openCodeResp.Error.Type,
			ErrorCode: openCodeResp.Error.Code,
			Message:   openCodeResp.Error.Message,
		}, true
	}

	// 2. Try OpenAI standard schema: {"error":{"message":"...","type":"...","code":"..."}}
	var openAIResp struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    any    `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(trimmed), &openAIResp); err == nil && openAIResp.Error.Message != "" {
		codeStr := ""
		if openAIResp.Error.Code != nil {
			codeStr = fmt.Sprintf("%v", openAIResp.Error.Code)
		}
		return parsedBody{
			ErrorType: openAIResp.Error.Type,
			ErrorCode: codeStr,
			Message:   openAIResp.Error.Message,
		}, true
	}

	// 3. Try direct {"message":"..."} or {"detail":"..."}
	var genericResp struct {
		Message string `json:"message"`
		Detail  string `json:"detail"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal([]byte(trimmed), &genericResp); err == nil {
		msg := genericResp.Message
		if msg == "" {
			msg = genericResp.Detail
		}
		if msg == "" {
			msg = genericResp.Error
		}
		if msg != "" {
			return parsedBody{Message: msg}, true
		}
	}

	return parsedBody{}, false
}

func isContextOverflow(msg string) bool {
	for _, ex := range overflowExclusions {
		if ex.MatchString(msg) {
			return false
		}
	}
	for _, re := range overflowPatterns {
		if re.MatchString(msg) {
			return true
		}
	}
	return false
}

// FindModelSuggestions finds nearest matching model names from known catalogs using Levenshtein distance.
func levenshteinDistance(s1, s2 string) int {
	r1, r2 := []rune(s1), []rune(s2)
	l1, l2 := len(r1), len(r2)
	matrix := make([][]int, l1+1)
	for i := range matrix {
		matrix[i] = make([]int, l2+1)
		matrix[i][0] = i
	}
	for j := 0; j <= l2; j++ {
		matrix[0][j] = j
	}

	for i := 1; i <= l1; i++ {
		for j := 1; j <= l2; j++ {
			cost := 1
			if r1[i-1] == r2[j-1] {
				cost = 0
			}
			matrix[i][j] = int(math.Min(
				float64(matrix[i-1][j]+1),
				math.Min(
					float64(matrix[i][j-1]+1),
					float64(matrix[i-1][j-1]+cost),
				),
			))
		}
	}
	return matrix[l1][l2]
}

// ClassifyOpenCodeError parses an arbitrary error into an OpenCode-aligned structured ClassifiedError.
func ClassifyOpenCodeError(err error, activeProvider string, activeModel string) ClassifiedError {
	if err == nil {
		return ClassifiedError{
			Kind:    ErrorKindGeneric,
			Message: "Unknown error",
		}
	}

	// 1. Cancellation check
	if errors.Is(err, context.Canceled) || strings.Contains(err.Error(), "context canceled") || strings.Contains(err.Error(), "UICancelledError") {
		return ClassifiedError{
			Kind:      ErrorKindCancelled,
			Title:     "Turn Cancelled",
			Badge:     "CANCELLED",
			Message:   "The turn was cancelled by the user.",
			Retryable: true,
		}
	}

	raw := err.Error()

	// 2. Maximum tool rounds / provider ignored the no-tools synthesis request.
	if errors.Is(err, applicationturn.ErrMaxRounds) {
		return ClassifiedError{
			Kind:    ErrorKindMaxRounds,
			Title:   "Maximum Tool Rounds Reached",
			Badge:   "MAX_ROUNDS",
			Message: "The model requested another tool after tool execution was disabled.",
			Suggestions: []string{
				"Run /new to start a fresh turn",
				"Increase agent.max_rounds if this task needs more tool rounds",
				"Ask the model to summarize its progress before continuing",
			},
			RawDetails: raw,
			Retryable:  true,
		}
	}

	if errors.Is(err, applicationturn.ErrToolDispatchUnavailable) {
		return ClassifiedError{
			Kind:    ErrorKindToolDispatch,
			Title:   "Tool Dispatch Unavailable",
			Badge:   "TOOL_DISPATCH",
			Message: "The model requested a tool, but no tools were available for this turn.",
			Suggestions: []string{
				"Check that the active runner has a registered tool set",
				"Verify the selected provider/model supports the configured tools",
				"Run /new after correcting the tool configuration",
			},
			RawDetails: raw,
			Retryable:  true,
		}
	}

	if errors.Is(err, applicationturn.ErrUnresolvedToolCall) {
		return ClassifiedError{
			Kind:    ErrorKindToolDispatch,
			Title:   "Unresolved Tool Call",
			Badge:   "TOOL_PROTOCOL",
			Message: "The model returned a tool call that the turn loop could not resolve.",
			Suggestions: []string{
				"Run /new to start a fresh turn",
				"Retry with a model that supports the configured tool protocol",
			},
			RawDetails: raw,
			Retryable:  true,
		}
	}

	// 3. Permission Denials
	if strings.Contains(raw, "permission denied") || strings.Contains(raw, "QuestionRejectedError") || strings.Contains(raw, "specified a rule") {
		return ClassifiedError{
			Kind:        ErrorKindPermissionDenied,
			Title:       "Permission Denied",
			Badge:       "DENIED",
			Message:     "Execution was blocked by permission policy or user rejection.",
			Suggestions: []string{"Change permission mode using Shift+Tab or run /mode always-approve"},
			RawDetails:  raw,
			Retryable:   false,
		}
	}

	// 3. Extract status code if present
	statusCode := 0
	if match := statusPattern.FindStringSubmatch(raw); len(match) > 1 {
		if parsed, parseErr := strconv.Atoi(match[1]); parseErr == nil {
			statusCode = parsed
		}
	}

	// 4. Extract embedded JSON payload
	var parsed parsedBody
	var hasParsed bool
	if idx := strings.Index(raw, "{"); idx != -1 {
		jsonStr := raw[idx:]
		if endIdx := strings.LastIndex(jsonStr, "}"); endIdx != -1 {
			jsonStr = jsonStr[:endIdx+1]
			parsed, hasParsed = parseAPIErrorPayload(jsonStr)
		}
	}

	effMessage := raw
	if hasParsed && parsed.Message != "" {
		effMessage = parsed.Message
	}

	// 5. HTML gateway responses (Cloudflare / proxy error page)
	if strings.Contains(raw, "<!doctype html") || strings.Contains(raw, "<html") || strings.Contains(raw, "<!DOCTYPE html") {
		if statusCode == 401 {
			return ClassifiedError{
				Kind:        ErrorKindAuthentication,
				Title:       "Gateway Authentication Required",
				Badge:       "401 UNAUTHORIZED",
				Message:     "Request was blocked by an upstream gateway or proxy. Your API key or session token is missing or expired.",
				Suggestions: []string{"Run /provider to configure your API key", "Or switch to OpenCode Free tier models: /provider opencode"},
				RawDetails:  raw,
				Code:        "401",
			}
		}
		if statusCode == 403 {
			return ClassifiedError{
				Kind:        ErrorKindForbidden,
				Title:       "Gateway Access Forbidden",
				Badge:       "403 FORBIDDEN",
				Message:     "Request was blocked by an upstream gateway or Cloudflare security policy.",
				Suggestions: []string{"Check your provider account and network settings", "Switch to an alternative model via /provider"},
				RawDetails:  raw,
				Code:        "403",
			}
		}
		return ClassifiedError{
			Kind:        ErrorKindServerOverloaded,
			Title:       "Gateway Error",
			Badge:       fmt.Sprintf("%d GATEWAY", statusCode),
			Message:     "The proxy gateway returned an HTML error page instead of an API response.",
			Suggestions: []string{"Wait a moment and retry", "Switch model provider using /provider"},
			RawDetails:  raw,
			Code:        strconv.Itoa(statusCode),
			Retryable:   true,
		}
	}

	// 6. Model Not Supported / Not Found (OpenCode ModelError, ProviderModelNotFoundError, or 404)
	isModelError := parsed.ErrorType == "ModelError" ||
		strings.Contains(raw, "is not supported") ||
		strings.Contains(raw, "ProviderModelNotFoundError") ||
		(statusCode == 404 && strings.Contains(strings.ToLower(raw), "model"))

	if isModelError {
		// Attempt to extract requested model name
		missingModel := activeModel
		if sub := modelErrorPattern.FindStringSubmatch(raw); len(sub) > 1 {
			missingModel = sub[1]
		} else if sub := modelNotFoundPattern.FindStringSubmatch(raw); len(sub) > 1 {
			missingModel = sub[1]
		}

		suggestions := []string{
			"Run /model to refresh the provider model catalog",
			"Run /provider to select or reconfigure the active provider",
		}

		cleanMsg := fmt.Sprintf("Model %q is not supported by provider %q.", missingModel, activeProvider)
		if activeProvider == "" {
			cleanMsg = fmt.Sprintf("Model %q is not supported by the active provider.", missingModel)
		}

		return ClassifiedError{
			Kind:        ErrorKindModelNotFound,
			Title:       "Model Not Supported",
			Badge:       "MODEL_NOT_FOUND",
			Message:     cleanMsg,
			Suggestions: suggestions,
			RawDetails:  raw,
			Code:        "404",
		}
	}

	// 7. Context Overflow (OpenCode 27 patterns, 413, or code context_length_exceeded)
	if statusCode == 413 || parsed.ErrorCode == "context_length_exceeded" || isContextOverflow(raw) || isContextOverflow(effMessage) {
		return ClassifiedError{
			Kind:    ErrorKindContextOverflow,
			Title:   "Context Window Overflow",
			Badge:   "CONTEXT_OVERFLOW",
			Message: "Input token count exceeds the maximum context length for this model.",
			Suggestions: []string{
				"Run /compact to summarize conversation history and free up tokens",
				"Run /new to start a fresh conversation session",
				"Switch to a high-context model via /provider (e.g. muse-spark or deepseek-v4)",
			},
			RawDetails: raw,
			Code:       "413",
			Retryable:  false,
		}
	}

	// 8. Authentication & Authorization (401 Unauthorized)
	if statusCode == 401 || parsed.ErrorType == "AuthError" || strings.Contains(strings.ToLower(raw), "unauthorized") || strings.Contains(strings.ToLower(raw), "invalid api key") {
		return ClassifiedError{
			Kind:    ErrorKindAuthentication,
			Title:   "Authentication Required",
			Badge:   "401 UNAUTHORIZED",
			Message: "Provider authentication failed. The API key is missing, invalid, or expired.",
			Suggestions: []string{
				"Run /provider to configure your API key",
				"Switch to OpenCode Free tier models (no key required): /provider opencode",
			},
			RawDetails: raw,
			Code:       "401",
		}
	}

	// 9. Forbidden (403 Forbidden)
	if statusCode == 403 || strings.Contains(strings.ToLower(raw), "forbidden") || strings.Contains(strings.ToLower(raw), "access_denied") {
		return ClassifiedError{
			Kind:    ErrorKindForbidden,
			Title:   "Access Forbidden",
			Badge:   "403 FORBIDDEN",
			Message: "Access to the requested model or API resource is forbidden for your account.",
			Suggestions: []string{
				"Check your account permissions or billing tier",
				"Switch to an available free model via /provider",
			},
			RawDetails: raw,
			Code:       "403",
		}
	}

	// 10. Quota & Billing Exceeded
	if parsed.ErrorCode == "insufficient_quota" || strings.Contains(raw, "insufficient_quota") || strings.Contains(raw, "quota exceeded") {
		return ClassifiedError{
			Kind:    ErrorKindQuotaExceeded,
			Title:   "Quota Exceeded",
			Badge:   "QUOTA_EXCEEDED",
			Message: "Your account quota has been exhausted. Check your plan and billing details.",
			Suggestions: []string{
				"Check billing details on your provider dashboard",
				"Switch to OpenCode free models: /provider opencode",
			},
			RawDetails: raw,
			Code:       "429",
		}
	}

	// 11. Rate Limit (429 Too Many Requests)
	if statusCode == 429 || parsed.ErrorType == "RateLimitError" || strings.Contains(strings.ToLower(raw), "rate limit") || strings.Contains(strings.ToLower(raw), "too many requests") {
		return ClassifiedError{
			Kind:    ErrorKindRateLimit,
			Title:   "Rate Limit Exceeded",
			Badge:   "429 RATE_LIMIT",
			Message: "Too many requests sent to the model provider in a short period.",
			Suggestions: []string{
				"Wait a few moments before sending another prompt",
				"Switch to another provider via /provider if limits persist",
			},
			RawDetails: raw,
			Code:       "429",
			Retryable:  true,
		}
	}

	// 12. Server Overloaded & Upstream Failures (500, 502, 503, 504)
	if statusCode == 500 || statusCode == 502 || statusCode == 503 || statusCode == 504 ||
		strings.Contains(raw, "server_is_overloaded") ||
		strings.Contains(raw, "server_error") ||
		strings.Contains(raw, "Upstream request failed") ||
		strings.Contains(strings.ToLower(raw), "service unavailable") ||
		strings.Contains(strings.ToLower(raw), "bad gateway") ||
		strings.Contains(strings.ToLower(raw), "gateway timeout") {
		codeStr := "5xx"
		if statusCode > 0 {
			codeStr = strconv.Itoa(statusCode)
		}
		return ClassifiedError{
			Kind:    ErrorKindServerOverloaded,
			Title:   "Provider Server Overloaded",
			Badge:   fmt.Sprintf("%s SERVER_ERROR", codeStr),
			Message: "The OpenCode upstream provider is temporarily overloaded or experiencing an outage.",
			Suggestions: []string{
				"This is usually a temporary outage — please retry in a moment",
				"Switch to an alternative model or provider using /provider",
			},
			RawDetails: raw,
			Code:       codeStr,
			Retryable:  true,
		}
	}

	// 13. Timeout & Stream Errors
	if strings.Contains(raw, "ProviderHeaderTimeoutError") ||
		strings.Contains(raw, "ProviderResponseStreamError") ||
		strings.Contains(strings.ToLower(raw), "deadline exceeded") ||
		strings.Contains(strings.ToLower(raw), "timeout") ||
		strings.Contains(raw, "unexpected EOF") ||
		strings.Contains(raw, "connection reset") {
		return ClassifiedError{
			Kind:    ErrorKindStreamTimeout,
			Title:   "Connection / Stream Timeout",
			Badge:   "TIMEOUT",
			Message: "The network connection timed out or the provider stream disconnected unexpectedly.",
			Suggestions: []string{
				"Check your internet connection and verify endpoint availability",
				"Retry your prompt",
			},
			RawDetails: raw,
			Retryable:  true,
		}
	}

	// 14. MCP Server Failures (MCPFailed)
	if strings.Contains(raw, "MCPFailed") || strings.Contains(raw, "MCP server") {
		serverName := "server"
		if m := regexp.MustCompile(`MCP server ["']?([^"'\s]+)["']? failed`).FindStringSubmatch(raw); len(m) > 1 {
			serverName = m[1]
		}
		return ClassifiedError{
			Kind:    ErrorKindMCPFailed,
			Title:   "MCP Server Failed",
			Badge:   "MCP_ERROR",
			Message: fmt.Sprintf("MCP server %q encountered a fatal failure. Note: MCP authentication is not supported yet.", serverName),
			Suggestions: []string{
				"Check MCP server configuration and process logs in " + appdirs.UserMCPLogsDisplay(),
				"Verify that all required environment variables for the MCP server are set",
			},
			RawDetails: raw,
		}
	}

	// 15. Config Directory Typo (ConfigDirectoryTypoError)
	if strings.Contains(raw, "ConfigDirectoryTypoError") || strings.Contains(raw, "is not valid. Rename the directory") {
		return ClassifiedError{
			Kind:        ErrorKindConfigTypo,
			Title:       "Config Directory Typo",
			Badge:       "CONFIG_TYPO",
			Message:     raw,
			Suggestions: []string{"Rename the invalid directory as indicated in the error message"},
			RawDetails:  raw,
		}
	}

	// 16. Config Errors (ConfigJsonError, ConfigInvalidError)
	if strings.Contains(raw, "ConfigJsonError") || strings.Contains(raw, "ConfigInvalidError") || strings.Contains(raw, "Configuration is invalid") {
		return ClassifiedError{
			Kind:    ErrorKindConfigInvalid,
			Title:   "Configuration Invalid",
			Badge:   "CONFIG_ERROR",
			Message: raw,
			Suggestions: []string{
				"Check your configuration file syntax (proton.toml or opencode.json)",
				"Ensure all provider keys and model configurations are properly structured",
			},
			RawDetails: raw,
		}
	}

	// 17. Tool Execution Failures
	if strings.Contains(raw, "tool execution failed") || strings.Contains(raw, "[COMMAND_FAILED]") || strings.Contains(raw, "[FILE_NOT_FOUND]") {
		return ClassifiedError{
			Kind:       ErrorKindToolFailed,
			Title:      "Tool Execution Error",
			Badge:      "TOOL_FAILED",
			Message:    effMessage,
			RawDetails: raw,
		}
	}

	// 18. Generic Fallback
	cleanMsg := effMessage
	if strings.HasPrefix(cleanMsg, "turn failed: ") {
		cleanMsg = strings.TrimPrefix(cleanMsg, "turn failed: ")
	}

	return ClassifiedError{
		Kind:        ErrorKindGeneric,
		Title:       "Operation Failed",
		Badge:       "ERROR",
		Message:     cleanMsg,
		Suggestions: []string{"Review the error details above or retry with /new"},
		RawDetails:  raw,
	}
}

// FormatErrorSummary produces a concise one-line summary of a classified error.
func FormatErrorSummary(c ClassifiedError) string {
	if c.Badge != "" {
		return fmt.Sprintf("[%s] %s: %s", c.Badge, c.Title, c.Message)
	}
	return fmt.Sprintf("%s: %s", c.Title, c.Message)
}
