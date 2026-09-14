package anthropic

import (
	"encoding/json"
	"strings"

	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

func anthropicHTTPError(status int, body []byte) *sdk.ProviderError {
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
	return sdk.NewProviderError("anthropic", status, code, message)
}
