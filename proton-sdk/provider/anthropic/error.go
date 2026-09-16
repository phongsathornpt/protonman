package anthropic

import (
	"encoding/json"
	"strings"

	"github.com/phongsathornpt/protonman/proton-sdk/domain"
)

func anthropicHTTPError(status int, body []byte) *domain.ProviderError {
	var payload struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	message := strings.TrimSpace(string(body))
	code := ""
	if json.Unmarshal(body, &payload) == nil {
		code = payload.Error.Type
		if strings.TrimSpace(payload.Error.Message) != "" {
			message = payload.Error.Message
		}
	}
	return domain.NewProviderError("anthropic", status, code, message)
}
