// Package model contains Proton CLI provider configuration and compatibility
// aliases. Provider-neutral model types are owned by proton-sdk.
package model

import sdk "github.com/projectTHORN/proton/proton-sdk"

type Role = sdk.Role

const (
	RoleSystem    = sdk.RoleSystem
	RoleUser      = sdk.RoleUser
	RoleAssistant = sdk.RoleAssistant
	RoleTool      = sdk.RoleTool
)

type ContentPartType = sdk.ContentPartType

const (
	ContentPartText  = sdk.ContentPartText
	ContentPartImage = sdk.ContentPartImage
)

type ContentPart = sdk.ContentPart
type ToolCall = sdk.ToolCall
type Message = sdk.Message

func CloneMessages(messages []Message) []Message { return sdk.CloneMessages(messages) }
