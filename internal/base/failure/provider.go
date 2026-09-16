package failure

import (
	"errors"
	"strings"

	"github.com/phongsathornpt/protonman/proton-sdk/domain"
)

// Classified describes a normalized application failure without exposing provider-specific enums.
type Classified struct {
	Code    Code
	Message string
}

// ClassifyProvider maps proton-sdk provider errors into the application failure catalog.
func ClassifyProvider(err error) (Classified, bool) {
	var providerErr *domain.ProviderError
	if !errors.As(err, &providerErr) {
		return Classified{}, false
	}
	code := CodeModelError
	switch providerErr.Kind {
	case domain.ErrorAuthentication:
		code = CodeModelAuthentication
	case domain.ErrorPermission:
		code = CodeModelPermission
	case domain.ErrorRateLimit:
		code = CodeModelRateLimited
	case domain.ErrorModelNotFound:
		code = CodeModelUnavailable
	case domain.ErrorContextLength:
		code = CodeModelContextLimit
	case domain.ErrorOverloaded:
		code = CodeModelOverloaded
	case domain.ErrorTransport:
		code = CodeNetworkUnavailable
	case domain.ErrorProtocol:
		code = CodeModelProtocol
	case domain.ErrorInvalidRequest:
		code = CodeModelInvalidRequest
	}
	return Classified{Code: code, Message: strings.TrimSpace(providerErr.Message)}, true
}

// Summary returns the stable short human-facing description for a known code.
func Summary(code Code) string {
	switch code {
	case CodeModelAuthentication:
		return "authentication failed"
	case CodeModelPermission:
		return "permission denied"
	case CodeModelRateLimited:
		return "rate limited"
	case CodeModelUnavailable:
		return "model unavailable"
	case CodeModelContextLimit:
		return "context limit exceeded"
	case CodeModelOverloaded:
		return "provider overloaded"
	case CodeNetworkUnavailable:
		return "network error"
	case CodeModelProtocol:
		return "provider protocol error"
	case CodeModelInvalidRequest:
		return "invalid model request"
	case CodeModelError:
		return "model error"
	default:
		return string(code)
	}
}
