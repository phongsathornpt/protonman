package tui

import "github.com/phongsathornpt/protonman/internal/adapter/in/tui/diagnostic"

type OpenCodeErrorKind = diagnostic.Kind

type ClassifiedError = diagnostic.Error

const (
	ErrorKindModelNotFound    = diagnostic.KindModelNotFound
	ErrorKindContextOverflow  = diagnostic.KindContextOverflow
	ErrorKindAuthentication   = diagnostic.KindAuthentication
	ErrorKindForbidden        = diagnostic.KindForbidden
	ErrorKindRateLimit        = diagnostic.KindRateLimit
	ErrorKindQuotaExceeded    = diagnostic.KindQuotaExceeded
	ErrorKindServerOverloaded = diagnostic.KindServerOverloaded
	ErrorKindStreamTimeout    = diagnostic.KindStreamTimeout
	ErrorKindInvalidPrompt    = diagnostic.KindInvalidPrompt
	ErrorKindMCPFailed        = diagnostic.KindMCPFailed
	ErrorKindConfigInvalid    = diagnostic.KindConfigInvalid
	ErrorKindConfigTypo       = diagnostic.KindConfigTypo
	ErrorKindToolFailed       = diagnostic.KindToolFailed
	ErrorKindToolDispatch     = diagnostic.KindToolDispatch
	ErrorKindPermissionDenied = diagnostic.KindPermissionDenied
	ErrorKindCancelled        = diagnostic.KindCancelled
	ErrorKindGeneric          = diagnostic.KindGeneric
)

func ClassifyOpenCodeError(err error, activeProvider, activeModel string) ClassifiedError {
	return diagnostic.Classify(err, activeProvider, activeModel)
}

func FormatErrorSummary(classified ClassifiedError) string {
	return diagnostic.FormatSummary(classified)
}
