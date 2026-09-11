package diagnostic

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/phongsathornpt/protonman/internal/app"
	"github.com/phongsathornpt/protonman/internal/app/appdirs"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

// Classify maps an arbitrary runtime/provider error into a structured presentation error.
func Classify(err error, activeProvider string, activeModel string) Error {
	if err == nil {
		return Error{
			Kind:    KindGeneric,
			Message: "Unknown error",
		}
	}

	// 1. Cancellation check
	if errors.Is(err, context.Canceled) || strings.Contains(err.Error(), "context canceled") || strings.Contains(err.Error(), "UICancelledError") {
		return Error{
			Kind:      KindCancelled,
			Title:     "Turn Cancelled",
			Badge:     "CANCELLED",
			Message:   "The turn was cancelled by the user.",
			Retryable: true,
		}
	}

	raw := err.Error()

	if errors.Is(err, app.ErrEmptyResponse) {
		return Error{
			Kind:    KindEmptyResponse,
			Title:   "Provider Returned No Output",
			Badge:   "EMPTY_RESPONSE",
			Message: "The provider completed the request without returning text or a tool call after bounded recovery attempts.",
			Suggestions: []string{
				"Retry the request; transient free-model streams can recover on a later call",
				"Switch models if the provider repeatedly returns an empty response",
			},
			RawDetails: raw,
			Retryable:  true,
		}
	}

	if errors.Is(err, sdk.ErrIncompleteStream) {
		lower := strings.ToLower(raw)
		if strings.Contains(lower, "produced no output before timeout") ||
			strings.Contains(lower, "stream became idle before completion") ||
			strings.Contains(lower, "stream exceeded maximum duration") {
			return Error{
				Kind:    KindStreamTimeout,
				Title:   "Provider Stream Timed Out",
				Badge:   "STREAM_TIMEOUT",
				Message: "The provider stream exceeded its bounded response window before completing.",
				Suggestions: []string{
					"Retry the request; transient provider stalls can recover",
					"Switch models if stream timeouts keep recurring",
				},
				RawDetails: raw,
				Retryable:  true,
			}
		}
		return Error{
			Kind:    KindStreamIncomplete,
			Title:   "Provider Stream Ended Early",
			Badge:   "STREAM_INCOMPLETE",
			Message: "The provider closed the response stream before sending a terminal completion event.",
			Suggestions: []string{
				"Retry the request; transient provider disconnects can recover",
				"Switch models if the stream repeatedly closes early",
			},
			RawDetails: raw,
			Retryable:  true,
		}
	}

	if errors.Is(err, app.ErrToolDispatchUnavailable) {
		return Error{
			Kind:    KindToolDispatch,
			Title:   "Tool Dispatch Unavailable",
			Badge:   "TOOL_DISPATCH",
			Message: "The model requested a tool, but no tools were available for this turn.",
			Suggestions: []string{
				"Check that the active runner has a registered tool set",
				"Verify the selected provider/model supports the configured tools",
				"Retry after correcting the tool configuration",
			},
			RawDetails: raw,
			Retryable:  true,
		}
	}

	if errors.Is(err, app.ErrUnresolvedToolCall) {
		return Error{
			Kind:    KindToolDispatch,
			Title:   "Unresolved Tool Call",
			Badge:   "TOOL_PROTOCOL",
			Message: "The model returned a tool call that the turn loop could not resolve.",
			Suggestions: []string{
				"Retry the request in a fresh turn",
				"Retry with a model that supports the configured tool protocol",
			},
			RawDetails: raw,
			Retryable:  true,
		}
	}

	// 3. Permission Denials
	if strings.Contains(raw, "permission denied") || strings.Contains(raw, "QuestionRejectedError") || strings.Contains(raw, "specified a rule") {
		return Error{
			Kind:        KindPermissionDenied,
			Title:       "Permission Denied",
			Badge:       "DENIED",
			Message:     "Execution was blocked by permission policy or user rejection.",
			Suggestions: []string{"Change permission mode using Shift+Tab"},
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
			return Error{
				Kind:        KindAuthentication,
				Title:       "Gateway Authentication Required",
				Badge:       "401 UNAUTHORIZED",
				Message:     "Request was blocked by an upstream gateway or proxy. Your API key or session token is missing or expired.",
				Suggestions: []string{"Run /provider to configure your API key", "Or switch to OpenCode Free tier models: /provider opencode"},
				RawDetails:  raw,
				Code:        "401",
			}
		}
		if statusCode == 403 {
			return Error{
				Kind:        KindForbidden,
				Title:       "Gateway Access Forbidden",
				Badge:       "403 FORBIDDEN",
				Message:     "Request was blocked by an upstream gateway or Cloudflare security policy.",
				Suggestions: []string{"Check your provider account and network settings", "Switch to an alternative model via /provider"},
				RawDetails:  raw,
				Code:        "403",
			}
		}
		return Error{
			Kind:        KindServerOverloaded,
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

		return Error{
			Kind:        KindModelNotFound,
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
		return Error{
			Kind:    KindContextOverflow,
			Title:   "Context Window Overflow",
			Badge:   "CONTEXT_OVERFLOW",
			Message: "Input token count exceeds the maximum context length for this model.",
			Suggestions: []string{
				"Run /compact to summarize conversation history and free up tokens",
				"Start a fresh conversation from the session launcher",
				"Switch to a high-context model via /provider (e.g. muse-spark or deepseek-v4)",
			},
			RawDetails: raw,
			Code:       "413",
			Retryable:  false,
		}
	}

	// 8. Authentication & Authorization (401 Unauthorized)
	if statusCode == 401 || parsed.ErrorType == "AuthError" || strings.Contains(strings.ToLower(raw), "unauthorized") || strings.Contains(strings.ToLower(raw), "invalid api key") {
		return Error{
			Kind:    KindAuthentication,
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
		return Error{
			Kind:    KindForbidden,
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
		return Error{
			Kind:    KindQuotaExceeded,
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
		return Error{
			Kind:    KindRateLimit,
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
		return Error{
			Kind:    KindServerOverloaded,
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

	// 13. Provider transport and stream failures. Keep this narrow: local
	// operation deadlines must not be presented as network failures.
	lowerRaw := strings.ToLower(raw)
	if strings.Contains(raw, "ProviderHeaderTimeoutError") ||
		strings.Contains(raw, "ProviderResponseStreamError") ||
		strings.Contains(lowerRaw, "client.timeout") ||
		strings.Contains(lowerRaw, "i/o timeout") ||
		strings.Contains(lowerRaw, "tls handshake timeout") ||
		strings.Contains(lowerRaw, "dial tcp") && strings.Contains(lowerRaw, "timeout") ||
		strings.Contains(raw, "unexpected EOF") ||
		strings.Contains(lowerRaw, "connection reset") {
		return Error{
			Kind:    KindStreamTimeout,
			Title:   "Connection / Stream Timeout",
			Badge:   "TIMEOUT",
			Message: "The network connection timed out or the provider stream disconnected unexpectedly.",
			Suggestions: []string{
				"Check endpoint availability and provider status",
				"Retry the request",
			},
			RawDetails: raw,
			Retryable:  true,
		}
	}

	// 14. Local/runtime deadline. This is deliberately separate from provider
	// transport failures so a bounded turn, tool, or agent operation never tells
	// the user to inspect their internet connection.
	if errors.Is(err, context.DeadlineExceeded) ||
		strings.Contains(lowerRaw, "deadline exceeded") ||
		strings.Contains(lowerRaw, "timed out") ||
		strings.Contains(lowerRaw, "timeout") {
		return Error{
			Kind:    KindRuntimeTimeout,
			Title:   "Operation Timeout",
			Badge:   "TIMEOUT",
			Message: "A bounded operation reached its execution deadline.",
			Suggestions: []string{
				"Retry the operation if it is still required",
				"Inspect the active tool or agent state before repeating work",
			},
			RawDetails: raw,
			Retryable:  true,
		}
	}

	// 15. MCP Server Failures (MCPFailed)
	if strings.Contains(raw, "MCPFailed") || strings.Contains(raw, "MCP server") {
		serverName := "server"
		if m := regexp.MustCompile(`MCP server ["']?([^"'\s]+)["']? failed`).FindStringSubmatch(raw); len(m) > 1 {
			serverName = m[1]
		}
		return Error{
			Kind:    KindMCPFailed,
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
		return Error{
			Kind:        KindConfigTypo,
			Title:       "Config Directory Typo",
			Badge:       "CONFIG_TYPO",
			Message:     raw,
			Suggestions: []string{"Rename the invalid directory as indicated in the error message"},
			RawDetails:  raw,
		}
	}

	// 16. Config Errors (ConfigJsonError, ConfigInvalidError)
	if strings.Contains(raw, "ConfigJsonError") || strings.Contains(raw, "ConfigInvalidError") || strings.Contains(raw, "Configuration is invalid") {
		return Error{
			Kind:    KindConfigInvalid,
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
		return Error{
			Kind:       KindToolFailed,
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

	return Error{
		Kind:        KindGeneric,
		Title:       "Operation Failed",
		Badge:       "ERROR",
		Message:     cleanMsg,
		Suggestions: []string{"Review the error details above, then retry"},
		RawDetails:  raw,
	}
}
