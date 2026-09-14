package runtime

import (
	"sync"
	"testing"

	"github.com/phongsathornpt/protonman/internal/adapter/out/config"
	"github.com/phongsathornpt/protonman/internal/core/modelclient"
	"github.com/phongsathornpt/protonman/internal/core/permission"
	sdk "github.com/phongsathornpt/protonman/proton-sdk"
)

// bindingTestFactory records root-memory session bindings requested by the TUI.
type bindingTestFactory struct {
	mu       sync.Mutex
	bindings [][2]string
}

func (f *bindingTestFactory) Build(modelclient.Request) sdk.LanguageModel { return nil }

func (f *bindingTestFactory) BindSession(sessionID, workspaceKey string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.bindings = append(f.bindings, [2]string{sessionID, workspaceKey})
}

func (f *bindingTestFactory) recorded() [][2]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([][2]string, len(f.bindings))
	copy(out, f.bindings)
	return out
}

// TestReconfigureRunnerBindsRootMemoryToActiveSession verifies that a session
// switch moves root durable-memory retrieval onto the newly active session
// instead of leaving it bound to the session captured at startup.
func TestReconfigureRunnerBindsRootMemoryToActiveSession(t *testing.T) {
	model := newTestBubbleModel(t, permission.ModeAsk, emptyTodoItems())
	factory := &bindingTestFactory{}
	model.application.ModelFactory = factory
	model.activeModel = "test-model"
	model.activeProvider = "openai"
	model.providers = map[string]config.ProviderConfig{"openai": {Name: "openai", APIKey: "sk-test-key", BaseURL: "https://api.openai.com/v1"}}

	model.sessionID = "session-one"
	model.workspaceKey = "ws-one"
	model.reconfigureRunner()

	model.sessionID = "session-two"
	model.workspaceKey = "ws-two"
	model.reconfigureRunner()

	got := factory.recorded()
	want := [][2]string{{"session-one", "ws-one"}, {"session-two", "ws-two"}}
	if len(got) != len(want) {
		t.Fatalf("bindings = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("binding[%d] = %#v, want %#v", i, got[i], want[i])
		}
	}
}
