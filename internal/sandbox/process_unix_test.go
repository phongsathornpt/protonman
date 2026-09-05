//go:build !windows

package sandbox

import (
	"context"
	"testing"
)

func TestCommandsUseIndependentProcessGroups(t *testing.T) {
	profile, err := NewProfile(NameOff, "")
	if err != nil {
		t.Fatalf("NewProfile() error = %v", err)
	}
	cmd, err := NewOSLauncher(profile).Command(context.Background(), t.TempDir(), "true")
	if err != nil {
		t.Fatalf("Command() error = %v", err)
	}
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setpgid {
		t.Fatalf("SysProcAttr = %#v, want Setpgid", cmd.SysProcAttr)
	}
}
