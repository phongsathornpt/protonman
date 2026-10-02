//go:build desktop || desktop_gio

package shell

import (
	"slices"
	"testing"

	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/controller"
	"github.com/phongsathornpt/protonman/internal/app"
)

func TestCommandSpecForACPAgentResolvesEnvironmentAtRuntime(t *testing.T) {
	t.Setenv("CUSTOM_ACP_TOKEN", "secret")
	spec := controller.CommandSpecForACPAgent(app.ACPAgentProfile{
		ID:      "custom",
		Command: "custom-acp",
		Args:    []string{"--stdio"},
		Env:     []string{"CUSTOM_ACP_TOKEN"},
	})
	if spec.Path != "custom-acp" || !slices.Equal(spec.Args, []string{"--stdio"}) {
		t.Fatalf("command = %#v", spec)
	}
	if !slices.Equal(spec.Env, []string{"CUSTOM_ACP_TOKEN=secret"}) {
		t.Fatalf("environment = %#v", spec.Env)
	}
}

func TestCommandSpecForACPAgentRetainsOverrideEnvironmentValues(t *testing.T) {
	spec := controller.CommandSpecForACPAgent(app.ACPAgentProfile{
		ID:      "custom",
		Command: "custom-acp",
		Env:     []string{"CUSTOM_ACP_TOKEN=override-secret"},
	})
	if !slices.Equal(spec.Env, []string{"CUSTOM_ACP_TOKEN=override-secret"}) {
		t.Fatalf("environment = %#v", spec.Env)
	}
}

func TestACPAgentProfileFromEditorValidatesAndScrubsEnvironment(t *testing.T) {
	profile, err := controller.ACPProfileFromEditor(" Reviewer ", " Review Agent ", " reviewer-acp ", `["--stdio"]`, `["TOKEN=secret","TOKEN"]`)
	if err != nil {
		t.Fatal(err)
	}
	if profile.ID != "reviewer" || profile.DisplayName != "Review Agent" || profile.Command != "reviewer-acp" {
		t.Fatalf("profile = %#v", profile)
	}
	if !slices.Equal(profile.Args, []string{"--stdio"}) || !slices.Equal(profile.Env, []string{"TOKEN"}) {
		t.Fatalf("profile lists = %#v", profile)
	}
}

func TestSessionConnectionUsesOwningAgent(t *testing.T) {
	snapshot := controller.Snapshot{
		ActiveAgentID:    "reviewer",
		Connection:       controller.ConnectionConnected,
		AgentConnections: map[string]controller.ConnectionPhase{controller.ProtonmanAgentID: controller.ConnectionReconnecting, "reviewer": controller.ConnectionConnected},
	}
	if got := sessionConnection(snapshot, controller.ProtonmanAgentID); got != controller.ConnectionReconnecting {
		t.Fatalf("proton connection = %q", got)
	}
	if got := sessionConnection(snapshot, "reviewer"); got != controller.ConnectionConnected {
		t.Fatalf("reviewer connection = %q", got)
	}
	if !anyAgentConnected(snapshot) {
		t.Fatal("a connected ACP agent was not detected")
	}
}

// The profile editor must survive a resync of the snapshot the user is editing.
// A resync happens on every turn boundary; losing the in-progress form there
// would discard a half-typed agent definition.
func TestSyncAgentProfileEditorsPreservesTheOpenEditor(t *testing.T) {
	sh := New(NewTheme("light"), Bindings{})
	widgets := sh.settingsComponent.AgentWidgets()
	snapshot := controller.Snapshot{
		Revision: 1,
		AgentProfiles: []app.ACPAgentProfile{
			{ID: "reviewer", DisplayName: "Reviewer", Command: "reviewer"},
		},
	}
	sh.syncAgentProfileEditors(snapshot)

	// Open an editor for an existing profile and type into it.
	widgets.EditorVisible = true
	widgets.EditorOriginalID = "reviewer"
	widgets.EditorKey = "reviewer"
	widgets.IDEditor.SetText("reviewer")
	widgets.NameEditor.SetText("My Reviewer")
	widgets.CommandEditor.SetText("reviewer-acp")

	// A later snapshot at a new revision must not tear the editor down.
	snapshot.Revision = 2
	sh.syncAgentProfileEditors(snapshot)
	if !widgets.EditorVisible {
		t.Fatal("resyncing the snapshot closed an open profile editor")
	}
	if got := widgets.NameEditor.Text(); got != "My Reviewer" {
		t.Fatalf("resync overwrote the in-progress editor: %q", got)
	}
	if widgets.ProfileButtons["reviewer"] == nil {
		t.Fatal("resync dropped the button for a profile that still exists")
	}
}

// When the profile being edited disappears from the snapshot, the editor must
// close rather than keep editing a row that no longer exists.
func TestSyncAgentProfileEditorsClosesEditorForRemovedProfile(t *testing.T) {
	sh := New(NewTheme("light"), Bindings{})
	widgets := sh.settingsComponent.AgentWidgets()
	sh.syncAgentProfileEditors(controller.Snapshot{
		Revision:      1,
		AgentProfiles: []app.ACPAgentProfile{{ID: "reviewer", DisplayName: "Reviewer"}},
	})
	widgets.EditorVisible = true
	widgets.EditorOriginalID = "reviewer"
	widgets.EditorKey = "reviewer"

	// The agent is gone in the next snapshot.
	sh.syncAgentProfileEditors(controller.Snapshot{Revision: 2})
	if widgets.EditorVisible {
		t.Fatal("editor stayed open after its profile was removed")
	}
	if widgets.EditorOriginalID != "" || widgets.EditorKey != "" {
		t.Fatalf("stale editor identity retained: original=%q key=%q", widgets.EditorOriginalID, widgets.EditorKey)
	}
	if widgets.ProfileButtons["reviewer"] != nil {
		t.Fatal("button for a removed profile was retained")
	}
}
