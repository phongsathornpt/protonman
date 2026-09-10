package diagnostic

import (
	"encoding/json"
	"fmt"
	"strings"
)

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
