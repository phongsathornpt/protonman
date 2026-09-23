package update

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/phongsathornpt/protonman/internal/base/buildinfo"
	"github.com/phongsathornpt/protonman/internal/base/envconfig"
	"github.com/phongsathornpt/protonman/internal/base/runtimepolicy"
	"github.com/phongsathornpt/protonman/internal/platform/appdirs"
)

type Status string

const (
	StatusUpToDate Status = "up-to-date"
	StatusUpdated  Status = "updated"
)

type Config struct {
	RequestedTag string
	Current      string
	BinDir       string
	StagingDir   string
	Client       *http.Client
}

type Result struct {
	Status   Status
	Previous string
	Current  string
	Tag      string
	Location string
}

func DefaultBinDir() string {
	if dir := strings.TrimSpace(envconfig.Value(envconfig.InstallDir)); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		home = strings.TrimSpace(os.Getenv("HOME"))
	}
	if strings.TrimSpace(home) == "" {
		return filepath.Join(".local", "bin")
	}
	return filepath.Join(home, ".local", "bin")
}

func Run(ctx context.Context, config Config) (Result, error) {
	client := config.Client
	if client == nil {
		client = newUpdateClient()
	}
	ctx, cancel := context.WithTimeout(ctx, runtimepolicy.UpdateOverallTimeout)
	defer cancel()
	current := strings.TrimSpace(config.Current)
	if current == "" {
		current = buildinfo.Version()
	}
	pin := strings.TrimSpace(config.RequestedTag)
	if pin == "" {
		pin = strings.TrimSpace(envconfig.Value(envconfig.PinnedVersion))
	}
	var tag string
	if pin != "" {
		validated, err := ValidateTag(pin)
		if err != nil {
			return Result{}, err
		}
		tag = validated
		if err := tagExists(ctx, client, tag); err != nil {
			return Result{}, err
		}
	} else {
		resolved, err := ResolveLatestTag(ctx, client)
		if err != nil {
			return Result{}, err
		}
		tag = resolved
	}
	if current != "" && current != "dev" && Compare(Plain(current), Plain(tag)) >= 0 {
		return Result{Status: StatusUpToDate, Previous: current, Current: current, Tag: tag}, nil
	}
	binDir := strings.TrimSpace(config.BinDir)
	if binDir == "" {
		binDir = DefaultBinDir()
	}
	archive, err := ArchiveName(tag)
	if err != nil {
		return Result{}, err
	}
	staging := strings.TrimSpace(config.StagingDir)
	var cleanup func()
	if staging == "" {
		dirs, err := appdirs.Resolve("")
		if err == nil && strings.TrimSpace(dirs.Downloads) != "" {
			staging = dirs.Downloads
		} else {
			staging = os.TempDir()
		}
		if err := os.MkdirAll(staging, 0o700); err != nil {
			return Result{}, fmt.Errorf("create staging directory: %w", err)
		}
		stagingDir, err := os.MkdirTemp(staging, "protonman-update-*")
		if err != nil {
			return Result{}, fmt.Errorf("create staging directory: %w", err)
		}
		staging = stagingDir
		cleanup = func() { _ = os.RemoveAll(stagingDir) }
	} else if err := os.MkdirAll(staging, 0o700); err != nil {
		return Result{}, fmt.Errorf("create staging directory: %w", err)
	}
	if cleanup != nil {
		defer cleanup()
	}
	archivePath := filepath.Join(staging, archive)
	checksumsPath := filepath.Join(staging, "checksums.txt")
	if err := downloadAsset(ctx, client, ReleaseAssetURL(tag, archive), archivePath, runtimepolicy.UpdateMaxDownloadBytes); err != nil {
		return Result{}, err
	}
	if err := downloadAsset(ctx, client, ReleaseAssetURL(tag, "checksums.txt"), checksumsPath, runtimepolicy.UpdateMaxChecksumsBytes); err != nil {
		return Result{}, err
	}
	if err := VerifyChecksum(archivePath, checksumsPath, archive); err != nil {
		return Result{}, err
	}
	staged, err := ExtractSingleBinary(archivePath, staging)
	if err != nil {
		return Result{}, err
	}
	if err := probeVersion(ctx, staged, Plain(tag)); err != nil {
		return Result{}, err
	}
	if err := InstallBinary(ctx, staged, binDir, Plain(tag)); err != nil {
		return Result{}, err
	}
	return Result{
		Status:   StatusUpdated,
		Previous: current,
		Current:  Plain(tag),
		Tag:      tag,
		Location: filepath.Join(binDir, BinaryName),
	}, nil
}
