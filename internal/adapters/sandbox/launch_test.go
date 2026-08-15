package sandbox

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	domainsandbox "github.com/projectTHORN/proton/internal/domain/sandbox"
)

func TestOffProfileUsesBareShell(t *testing.T) {
	profile, err := domainsandbox.NewProfile(domainsandbox.NameOff, "")
	if err != nil {
		t.Fatalf("NewProfile() error = %v", err)
	}
	launcher := NewOSLauncher(profile)
	cmd, err := launcher.Command(context.Background(), t.TempDir(), "echo hi")
	if err != nil {
		t.Fatalf("Command() error = %v", err)
	}
	if filepath.Base(cmd.Path) == "bwrap" || filepath.Base(cmd.Path) == "sandbox-exec" {
		t.Fatalf("off profile wrapped command: %s", cmd.Path)
	}
}

func TestConfiningProfileFailsClosedWhenToolsMissing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows launcher is fail-closed by design")
	}
	profile, err := domainsandbox.NewProfile(domainsandbox.NameStrict, t.TempDir())
	if err != nil {
		t.Fatalf("NewProfile() error = %v", err)
	}
	launcher := &OSLauncher{
		Profile: profile,
		LookPath: func(string) (string, error) {
			return "", errors.New("missing")
		},
	}
	_, err = launcher.Command(context.Background(), t.TempDir(), "echo hi")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Command() error = %v, want unavailable", err)
	}
}

func TestSeatbeltProfileDeniesNetworkWhenRestricted(t *testing.T) {
	profile, err := domainsandbox.NewProfile(domainsandbox.NameStrict, "/tmp/ws")
	if err != nil {
		t.Fatalf("NewProfile() error = %v", err)
	}
	text := seatbeltProfile(profile, "/tmp/ws")
	if !strings.Contains(text, "(deny network*)") {
		t.Fatalf("seatbelt profile missing network deny: %s", text)
	}
	if !strings.Contains(text, "/tmp/ws") {
		t.Fatalf("seatbelt profile missing workspace: %s", text)
	}
}

func TestBwrapUsesUnshareNetWhenRestricted(t *testing.T) {
	profile, err := domainsandbox.NewProfile(domainsandbox.NameStrict, "/tmp/ws")
	if err != nil {
		t.Fatalf("NewProfile() error = %v", err)
	}
	cmd := bwrapCommand(context.Background(), "bwrap", profile, "/tmp/ws", "true")
	joined := strings.Join(cmd.Args, " ")
	if !strings.Contains(joined, "--unshare-net") {
		t.Fatalf("bwrap args missing --unshare-net: %s", joined)
	}
}
