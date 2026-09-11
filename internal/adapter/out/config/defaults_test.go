package config

import (
	"testing"

	"github.com/phongsathornpt/protonman/internal/base/runtimepolicy"
	"github.com/phongsathornpt/protonman/internal/core/permission"
)

func TestDefaultSnapshotUsesRuntimePolicy(t *testing.T) {
	snapshot := DefaultSnapshot()
	if snapshot.Mode != permission.ModeAsk || snapshot.Permission.Default != permission.ActionAsk {
		t.Fatalf("permission defaults = mode:%v action:%v", snapshot.Mode, snapshot.Permission.Default)
	}
	if snapshot.Agent.MaxToolCalls != runtimepolicy.TurnMaxToolCalls || snapshot.Agent.MaxLiveSubagents != runtimepolicy.AgentMaxLive {
		t.Fatalf("agent defaults = %+v", snapshot.Agent)
	}
	if snapshot.Runtime.ModelRequestTimeout != runtimepolicy.ModelRequestTimeout {
		t.Fatalf("model request timeout = %v, want %v", snapshot.Runtime.ModelRequestTimeout, runtimepolicy.ModelRequestTimeout)
	}
}

func TestDefaultSnapshotReturnsIndependentMutableState(t *testing.T) {
	first := DefaultSnapshot()
	second := DefaultSnapshot()
	first.Providers["mutated"] = ProviderConfig{Name: "mutated"}
	first.Agent.Subagents["strength"] = SubagentModelConfig{Model: "mutated"}
	first.Permission.Rules = append(first.Permission.Rules, permission.Rule{Action: permission.ActionDeny, Tool: permission.ToolBash})
	first.Provenance[FieldModelDefault] = SourceProject
	if len(second.Providers) != 0 || len(second.Agent.Subagents) != 0 || len(second.Permission.Rules) != 0 {
		t.Fatalf("default snapshots share mutable state: %+v", second)
	}
	if second.Provenance[FieldModelDefault] != SourceDefault {
		t.Fatalf("provenance leaked across snapshots: %v", second.Provenance)
	}
}
