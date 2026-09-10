package providerutil

import (
	"net/http"
	"strings"
)

const SessionIDHeader = "X-Session-Id"

// ApplySessionID applies the canonical request-scoped session identity after
// provider custom headers, so arbitrary headers cannot silently override it.
func ApplySessionID(headers http.Header, sessionID string) {
	if headers == nil {
		return
	}
	if sessionID = strings.TrimSpace(sessionID); sessionID != "" {
		headers.Set(SessionIDHeader, sessionID)
	}
}
