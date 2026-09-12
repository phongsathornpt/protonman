package turn

type toolDispatchReason string

const (
	toolDispatchEnabled            toolDispatchReason = "enabled"
	toolDispatchDisabledMaxCalls   toolDispatchReason = "max_tool_calls"
	toolDispatchDisabledNoProgress toolDispatchReason = "no_progress"
	toolDispatchDisabledNoTools    toolDispatchReason = "no_tools"
	toolDispatchDisabledModelTools toolDispatchReason = "model_tools_unsupported"
)

type toolDispatchState struct {
	reason              toolDispatchReason
	remainingToolCalls  int
	providerToCanonical map[string]string
	canonicalNames      map[string]struct{}
}

func (s toolDispatchState) canonicalToolName(name string) string {
	if canonical, ok := s.providerToCanonical[name]; ok {
		return canonical
	}
	if _, ok := s.canonicalNames[name]; ok {
		return name
	}
	return name
}

func (s toolDispatchState) enabled() bool {
	return s.reason == toolDispatchEnabled
}
