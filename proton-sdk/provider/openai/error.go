package openai

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	sdk "github.com/phongsathornpt/proton/proton-sdk"
)

type providerErrorPayload struct {
	Message  string
	Type     string
	Code     string
	Metadata map[string]any
}

func parseProviderErrorPayload(body []byte) providerErrorPayload {
	var envelope struct {
		Error struct {
			Message  string         `json:"message"`
			Type     string         `json:"type"`
			Code     any            `json:"code"`
			Metadata map[string]any `json:"metadata"`
		} `json:"error"`
		Metadata map[string]any `json:"metadata"`
	}
	payload := providerErrorPayload{Message: strings.TrimSpace(string(body))}
	if json.Unmarshal(body, &envelope) != nil {
		return payload
	}
	payload.Message = firstNonEmpty(envelope.Error.Message, payload.Message)
	payload.Type = strings.TrimSpace(envelope.Error.Type)
	if envelope.Error.Code != nil {
		payload.Code = strings.TrimSpace(fmt.Sprint(envelope.Error.Code))
	}
	if payload.Code == "" {
		payload.Code = payload.Type
	}
	payload.Metadata = mergeErrorMetadata(envelope.Metadata, envelope.Error.Metadata)
	return payload
}

func mergeErrorMetadata(values ...map[string]any) map[string]any {
	var merged map[string]any
	for _, value := range values {
		for key, item := range value {
			if merged == nil {
				merged = map[string]any{}
			}
			merged[key] = item
		}
	}
	return merged
}

func providerError(provider string, status int, body []byte, headers http.Header) *sdk.ProviderError {
	payload := parseProviderErrorPayload(body)
	err := sdk.NewProviderError(provider, status, payload.Code, payload.Message)
	if strings.EqualFold(provider, "opencode") {
		classifyOpenCodeRateLimit(err, payload, headers)
	} else if err.Kind == sdk.ErrorRateLimit {
		err.RateLimit = sdk.ParseRateLimitHeaders(headers, time.Now())
	}
	return err
}

func providerStreamError(provider, code, errorType, message string, metadata map[string]any) *sdk.ProviderError {
	payload := providerErrorPayload{Code: firstNonEmpty(code, errorType), Type: errorType, Message: message, Metadata: metadata}
	err := sdk.NewProviderError(provider, 0, payload.Code, payload.Message)
	if strings.EqualFold(provider, "opencode") {
		classifyOpenCodeRateLimit(err, payload, nil)
	}
	return err
}

func classifyOpenCodeRateLimit(err *sdk.ProviderError, payload providerErrorPayload, headers http.Header) {
	if err == nil {
		return
	}
	value := strings.ToLower(strings.Join([]string{payload.Type, payload.Code, payload.Message}, " "))
	info := sdk.ParseRateLimitHeaders(headers, time.Now())
	if info == nil {
		info = &sdk.RateLimitInfo{}
	}
	limitName := strings.ToLower(strings.TrimSpace(metadataString(payload.Metadata, "limitName", "limit_name", "limit")))
	info.LimitName = limitName

	switch {
	case strings.Contains(value, "freeusagelimiterror"), strings.Contains(value, "free usage limit"):
		info.Kind = sdk.RateLimitFreeUsage
	case strings.Contains(value, "gousagelimiterror"), strings.Contains(value, "go usage limit"):
		switch {
		case strings.Contains(limitName, "5") && strings.Contains(limitName, "hour"):
			info.Kind = sdk.RateLimitGoFiveHour
		case strings.Contains(limitName, "week"):
			info.Kind = sdk.RateLimitGoWeekly
		case strings.Contains(limitName, "month"):
			info.Kind = sdk.RateLimitGoMonthly
		default:
			info.Kind = sdk.RateLimitUnknownQuota
		}
		info.Scope = sdk.RateLimitScopeAccount
	case strings.Contains(value, "provider rate limit"), strings.Contains(value, "upstream rate limit"):
		info.Kind = sdk.RateLimitProvider
		info.Scope = sdk.RateLimitScopeProvider
	case err.StatusCode == http.StatusTooManyRequests || err.Kind == sdk.ErrorRateLimit:
		info.Kind = sdk.RateLimitTransient
		info.Scope = sdk.RateLimitScopeRequest
	default:
		return
	}
	err.Kind = sdk.ErrorRateLimit
	err.Retryable = info.Kind == sdk.RateLimitTransient || info.Kind == sdk.RateLimitProvider
	err.RateLimit = info
}

func metadataString(metadata map[string]any, keys ...string) string {
	for _, key := range keys {
		for candidate, value := range metadata {
			if strings.EqualFold(candidate, key) {
				return fmt.Sprint(value)
			}
		}
	}
	return ""
}
