package protonsdk

import (
	"github.com/phongsathornpt/protonman/proton-sdk/usecase"
)

// NewRegistry creates a registry resolving scoped model IDs such as "anthropic/claude-sonnet-5".
func NewRegistry() *Registry {
	return usecase.NewRegistry()
}

// WrapLanguageModel applies middleware in declaration order, with the first
// middleware acting as the outermost wrapper.
func WrapLanguageModel(model LanguageModel, middleware ...Middleware) LanguageModel {
	return usecase.WrapLanguageModel(model, middleware...)
}
