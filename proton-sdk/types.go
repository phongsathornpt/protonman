// Package protonsdk defines the provider-neutral model boundary used by Protonman agents.
// It serves as a unified backward-compatible facade over domain, port, and usecase layers.
package protonsdk

import (
	"github.com/phongsathornpt/protonman/proton-sdk/domain"
	"github.com/phongsathornpt/protonman/proton-sdk/port"
	"github.com/phongsathornpt/protonman/proton-sdk/usecase"
)

// Model types
type LanguageModel = port.LanguageModel
type ModelCapabilities = domain.ModelCapabilities
type RequestRequirements = domain.RequestRequirements
type ModelMetadata = domain.ModelMetadata
type MetadataModel = port.MetadataModel
type TokenLimits = domain.TokenLimits
type TokenLimitsModel = port.TokenLimitsModel
type ContextWindowModel = port.ContextWindowModel
type ProviderOptions = domain.ProviderOptions
type ProviderMetadata = domain.ProviderMetadata

// Message types
type Role = domain.Role
type ContentPartType = domain.ContentPartType
type ContentPart = domain.ContentPart
type ReasoningEffort = domain.ReasoningEffort
type Message = domain.Message

// Request types
type ToolChoice = domain.ToolChoice
type ModelOptions = domain.ModelOptions
type RequestMetadata = domain.RequestMetadata
type Request = domain.Request
type ToolCall = domain.ToolCall
type Tool = domain.Tool
type ToolResult = domain.ToolResult
type SchemaValidator = port.SchemaValidator
type ToolSchemaValidator = usecase.ToolSchemaValidator

// Stream types
type FinishReason = domain.FinishReason
type EventKind = domain.EventKind
type Event = domain.Event
type Stream = port.Stream
type Usage = domain.Usage
type Response = domain.Response
type StepResult = domain.StepResult
type ResponseAccumulator = usecase.ResponseAccumulator

// Error types
type ErrorKind = domain.ErrorKind
type ProviderError = domain.ProviderError
type RateLimitKind = domain.RateLimitKind
type RateLimitScope = domain.RateLimitScope
type RateLimitInfo = domain.RateLimitInfo
type RetryPolicy = domain.RetryPolicy
type RetryDecision = domain.RetryDecision
type RetryPhase = domain.RetryPhase
type RetryEvent = domain.RetryEvent
type RetryObserver = domain.RetryObserver

// Registry & Middleware types
type ModelFactory = port.ModelFactory
type Registry = usecase.Registry
type StreamFunc = port.StreamFunc
type Middleware = port.Middleware
type MiddlewareFunc = port.MiddlewareFunc

