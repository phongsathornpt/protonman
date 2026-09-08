# Installing Protonman

Protonman release binaries are distributed through GitHub Releases. The supported `install.sh` path targets Linux and macOS on amd64 or arm64.

## Public release install

For the latest stable public release:

```sh
curl -fsSL https://github.com/phongsathornpt/protonman/releases/latest/download/install.sh | sh
```

The installer places `protonman` in `~/.local/bin` by default and verifies the release SHA-256 checksum before executing or installing the downloaded binary.

Verify the install:

```sh
protonman --version
```

If `~/.local/bin` is not on `PATH`, add it through your shell configuration.

## Install an exact version

```sh
curl -fsSL https://github.com/phongsathornpt/protonman/releases/latest/download/install.sh \
  | sh -s -- --version v1.2.3
```

## Custom install directory

```sh
curl -fsSL https://github.com/phongsathornpt/protonman/releases/latest/download/install.sh \
  | sh -s -- --bin-dir "$HOME/bin"
```

The equivalent environment variables are:

```text
PROTONMAN_VERSION
PROTONMAN_INSTALL_DIR
```

The installer never invokes `sudo` automatically. Choose a writable `--bin-dir` if the default is unsuitable.

## Private repository access

When Protonman is private, obtain `install.sh` from an authenticated checkout. Set `GITHUB_TOKEN` or `GH_TOKEN` before running it so the installer can resolve and download private release assets through the GitHub Releases API.

```sh
export GITHUB_TOKEN='<token with repository contents read access>'
./install.sh --version v1.2.3
```

The token is sent only to GitHub API/download requests. It is not written to Protonman configuration or release artifacts.

## Upgrading

Run the installer again. A verified binary is staged in the destination directory and then moved over the existing executable.

```sh
./install.sh
```

Pin a release when reproducibility matters:

```sh
./install.sh --version v1.3.0
```

## Uninstalling

Remove only the executable:

```sh
rm "$HOME/.local/bin/protonman"
```

Runtime data is intentionally separate and is not deleted by uninstalling the binary. Protonman runtime state lives under `~/.protonman/`.

Delete runtime data only when you explicitly want to remove configuration, sessions, checkpoints, skills, and logs:

```sh
rm -rf "$HOME/.protonman"
```

## Supported release targets

| OS | Architecture | Archive |
| :--- | :--- | :--- |
| Linux | amd64 | `.tar.gz` |
| macOS | arm64 | `.tar.gz` |

Other OS/architecture combinations are not published or supported by the release installer.

## Verification and failure behavior

The installer fails closed when the platform is unsupported, the requested tag is invalid, an asset is missing, no SHA-256 tool is available, the checksum differs, the archive layout is unexpected, or the embedded binary version does not match the requested release.

It supports `sha256sum`, `shasum -a 256`, or `openssl dgst -sha256`. A downloaded binary is not executed until its archive checksum has passed verification.

## Go install

A versioned Go install is also supported:

```sh
go install github.com/phongsathornpt/protonman/cmd/protonman@v1.2.3
```

Go build metadata provides the version fallback for this installation path.
