package providerutil

import (
	"net/http"
	"time"

	"github.com/phongsathornpt/protonman/pkg/proton-sdk/domain"
)

// ParseRateLimitHeaders extracts rate limit information from HTTP response headers.
func ParseRateLimitHeaders(headers http.Header, now time.Time) *domain.RateLimitInfo {
	return domain.ParseRateLimitHeaders(headers, now)
}
