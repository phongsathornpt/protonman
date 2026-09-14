package acp

import "testing"

func TestSessionListTitle(t *testing.T) {
	tests := []struct {
		name          string
		id            string
		workspaceName string
		preview       string
		want          string
	}{
		{
			name:          "conversation preview wins",
			id:            "workspace-demo-123",
			workspaceName: "demo",
			preview:       "Refactor the desktop shell",
			want:          "Refactor the desktop shell",
		},
		{
			name:          "workspace is fallback",
			id:            "workspace-demo-123",
			workspaceName: " demo ",
			want:          "demo",
		},
		{
			name: "session id is final fallback",
			id:   "workspace-demo-123",
			want: "Session workspace-demo-123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sessionListTitle(tt.id, tt.workspaceName, tt.preview); got != tt.want {
				t.Fatalf("sessionListTitle(%q, %q, %q) = %q, want %q", tt.id, tt.workspaceName, tt.preview, got, tt.want)
			}
		})
	}
}
