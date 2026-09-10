package protonsdk

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

type RateLimitKind string

const (
	RateLimitUnknown      RateLimitKind = ""
	RateLimitTransient    RateLimitKind = "transient"
	RateLimitProvider     RateLimitKind = "provider"
	RateLimitFreeUsage    RateLimitKind = "free_usage"
	RateLimitGoFiveHour   RateLimitKind = "go_5_hour"
	RateLimitGoWeekly     RateLimitKind = "go_weekly"
	RateLimitGoMonthly    RateLimitKind = "go_monthly"
	RateLimitUnknownQuota RateLimitKind = "quota"
)

type RateLimitScope string

const (
	RateLimitScopeUnknown  RateLimitScope = ""
	RateLimitScopeRequest  RateLimitScope = "request"
	RateLimitScopeModel    RateLimitScope = "model"
	RateLimitScopeProvider RateLimitScope = "provider"
	RateLimitScopeAccount  RateLimitScope = "account"
	RateLimitScopeIP       RateLimitScope = "ip"
)

type RateLimitInfo struct {
	Kind       RateLimitKind  `json:"kind,omitempty"`
	Scope      RateLimitScope `json:"scope,omitempty"`
	LimitName  string         `json:"limit_name,omitempty"`
	RetryAfter time.Duration  `json:"retry_after,omitempty"`
	ResetAt    time.Time      `json:"reset_at,omitempty"`
	Limit      *int64         `json:"limit,omitempty"`
	Remaining  *int64         `json:"remaining,omitempty"`
}

func ParseRateLimitHeaders(headers http.Header, now time.Time) *RateLimitInfo {
	if len(headers) == 0 {
		return nil
	}
	info := &RateLimitInfo{}
	if value := strings.TrimSpace(headerValue(headers, "Retry-After")); value != "" {
		if seconds, err := strconv.ParseInt(value, 10, 64); err == nil && seconds >= 0 {
			info.RetryAfter = time.Duration(seconds) * time.Second
			info.ResetAt = now.Add(info.RetryAfter)
		} else if when, err := http.ParseTime(value); err == nil {
			info.ResetAt = when
			if when.After(now) {
				info.RetryAfter = when.Sub(now)
			}
		}
	}
	info.Limit = firstHeaderInt(headers, "X-RateLimit-Limit", "X-RateLimit-Limit-Requests", "Anthropic-RateLimit-Requests-Limit", "Anthropic-RateLimit-Tokens-Limit")
	info.Remaining = firstHeaderInt(headers, "X-RateLimit-Remaining", "X-RateLimit-Remaining-Requests", "Anthropic-RateLimit-Requests-Remaining", "Anthropic-RateLimit-Tokens-Remaining")
	if info.ResetAt.IsZero() {
		for _, key := range []string{"X-RateLimit-Reset", "X-RateLimit-Reset-Requests", "X-RateLimit-Reset-Tokens", "Anthropic-RateLimit-Requests-Reset", "Anthropic-RateLimit-Tokens-Reset"} {
			if when, ok := parseRateLimitReset(headerValue(headers, key), now); ok {
				info.ResetAt = when
				if when.After(now) {
					info.RetryAfter = when.Sub(now)
				}
				break
			}
		}
	}
	if info.RetryAfter == 0 && info.ResetAt.IsZero() && info.Limit == nil && info.Remaining == nil {
		return nil
	}
	return info
}

func firstHeaderInt(headers http.Header, keys ...string) *int64 {
	for _, key := range keys {
		value := strings.TrimSpace(headerValue(headers, key))
		if value == "" {
			continue
		}
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err == nil {
			return &parsed
		}
	}
	return nil
}

func parseRateLimitReset(value string, now time.Time) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	if seconds, err := strconv.ParseFloat(value, 64); err == nil {
		if seconds > 1_000_000_000 {
			return time.Unix(int64(seconds), int64((seconds-float64(int64(seconds)))*float64(time.Second))), true
		}
		if seconds >= 0 {
			return now.Add(time.Duration(seconds * float64(time.Second))), true
		}
	}
	if duration, err := time.ParseDuration(value); err == nil && duration >= 0 {
		return now.Add(duration), true
	}
	if when, err := http.ParseTime(value); err == nil {
		return when, true
	}
	if when, err := time.Parse(time.RFC3339, value); err == nil {
		return when, true
	}
	return time.Time{}, false
}

func headerValue(headers http.Header, key string) string {
	for candidate, values := range headers {
		if !strings.EqualFold(candidate, key) || len(values) == 0 {
			continue
		}
		return values[0]
	}
	return ""
}
