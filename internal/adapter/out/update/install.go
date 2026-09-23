package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/phongsathornpt/protonman/internal/base/buildinfo"
	"github.com/phongsathornpt/protonman/internal/base/runtimepolicy"
)

const BinaryName = "protonman"

func SupportedPlatform(goos, goarch string) error {
	switch goos + "/" + goarch {
	case "linux/amd64", "darwin/arm64":
		return nil
	default:
		return fmt.Errorf("unsupported platform: %s/%s (supported: linux/amd64, darwin/arm64)", goos, goarch)
	}
}

func ArchiveName(tag string) (string, error) {
	if err := SupportedPlatform(runtime.GOOS, runtime.GOARCH); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s_%s_%s_%s.tar.gz", BinaryName, Plain(tag), runtime.GOOS, runtime.GOARCH), nil
}

func ReleaseAssetURL(tag, asset string) string {
	return githubWeb + "/" + Repository + "/releases/download/" + tag + "/" + asset
}

func downloadAsset(ctx context.Context, client *http.Client, url, dest string, maxBytes int64) error {
	if err := updatePolicy().AllowURL(url); err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build download request: %w", err)
	}
	request.Header.Set("User-Agent", buildinfo.WebUserAgent())
	request.Header.Set("Accept", "application/octet-stream")
	if token := authToken(); token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: unexpected status %d", url, response.StatusCode)
	}
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("create download destination: %w", err)
	}
	defer out.Close()
	written, err := io.Copy(out, io.LimitReader(response.Body, maxBytes+1))
	if err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	if written > maxBytes {
		return fmt.Errorf("download %s exceeds %d bytes", url, maxBytes)
	}
	return nil
}

func parseChecksums(data []byte) (map[string]string, error) {
	entries := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 {
			continue
		}
		if len(fields[0]) != 64 {
			continue
		}
		if _, err := hex.DecodeString(fields[0]); err != nil {
			continue
		}
		entries[fields[1]] = strings.ToLower(fields[0])
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("checksums file contains no entries")
	}
	return entries, nil
}

func VerifyChecksum(archivePath, checksumsPath, archiveName string) error {
	checksumData, err := os.ReadFile(checksumsPath)
	if err != nil {
		return fmt.Errorf("read checksums: %w", err)
	}
	entries, err := parseChecksums(checksumData)
	if err != nil {
		return err
	}
	expected, ok := entries[archiveName]
	if !ok {
		return fmt.Errorf("checksum entry not found for %s", archiveName)
	}
	handle, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("open archive: %w", err)
	}
	defer handle.Close()
	sum := sha256.New()
	if _, err := io.Copy(sum, io.LimitReader(handle, runtimepolicy.UpdateMaxDownloadBytes+1)); err != nil {
		return fmt.Errorf("hash archive: %w", err)
	}
	if actual := hex.EncodeToString(sum.Sum(nil)); actual != expected {
		return fmt.Errorf("checksum mismatch for %s", archiveName)
	}
	return nil
}

func ExtractSingleBinary(archivePath, destDir string) (string, error) {
	handle, err := os.Open(archivePath)
	if err != nil {
		return "", fmt.Errorf("open archive: %w", err)
	}
	defer handle.Close()
	gz, err := gzip.NewReader(io.LimitReader(handle, runtimepolicy.UpdateMaxDownloadBytes+1))
	if err != nil {
		return "", fmt.Errorf("open release archive: %w", err)
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	var extracted string
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("read release archive: %w", err)
		}
		if extracted != "" {
			return "", fmt.Errorf("unexpected archive contents: more than one entry")
		}
		if header.Name != BinaryName || !header.FileInfo().Mode().IsRegular() {
			return "", fmt.Errorf("unexpected archive contents: %q", header.Name)
		}
		if header.Size < 0 || header.Size > runtimepolicy.UpdateMaxDownloadBytes {
			return "", fmt.Errorf("unexpected archive entry size for %q", header.Name)
		}
		out, err := os.OpenFile(filepath.Join(destDir, BinaryName), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			return "", fmt.Errorf("create staged binary: %w", err)
		}
		if _, err := io.Copy(out, io.LimitReader(reader, runtimepolicy.UpdateMaxDownloadBytes+1)); err != nil {
			_ = out.Close()
			return "", fmt.Errorf("extract release archive: %w", err)
		}
		if err := out.Close(); err != nil {
			return "", fmt.Errorf("close staged binary: %w", err)
		}
		extracted = filepath.Join(destDir, BinaryName)
	}
	if extracted == "" {
		return "", fmt.Errorf("release archive did not contain %s", BinaryName)
	}
	if err := os.Chmod(extracted, 0o755); err != nil {
		return "", fmt.Errorf("make staged binary executable: %w", err)
	}
	return extracted, nil
}

func probeVersion(ctx context.Context, binary, wantPlain string) error {
	ctx, cancel := context.WithTimeout(ctx, runtimepolicy.UpdateProbeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "--version")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("downloaded binary failed version verification: %w", err)
	}
	want := buildinfo.Name + " " + wantPlain
	if got := strings.TrimSpace(stdout.String()); got != want {
		return fmt.Errorf("downloaded binary version mismatch: got %q, want %q", got, want)
	}
	return nil
}

func InstallBinary(ctx context.Context, staged, binDir, wantPlain string) error {
	info, err := os.Stat(binDir)
	if err == nil && !info.IsDir() {
		return fmt.Errorf("install path is not a directory: %s", binDir)
	}
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return fmt.Errorf("could not create install directory %s: %w", binDir, err)
	}
	tmp, err := os.CreateTemp(binDir, "."+BinaryName+".install.*")
	if err != nil {
		return fmt.Errorf("could not stage Protonman binary: %w", err)
	}
	tmpName := tmp.Name()
	_ = tmp.Close()
	_ = os.Remove(tmpName)
	if err := copyFile(staged, tmpName, 0o755); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("could not stage Protonman binary: %w", err)
	}
	target := filepath.Join(binDir, BinaryName)
	if err := os.Rename(tmpName, target); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("could not install Protonman: %w", err)
	}
	if err := probeVersion(ctx, target, wantPlain); err != nil {
		return fmt.Errorf("installed binary failed verification: %w", err)
	}
	return nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chmod(dst, mode)
}
