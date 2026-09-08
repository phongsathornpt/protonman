# Releasing Protonman

Git tags are the source of truth for release versions. Stable releases use
`vMAJOR.MINOR.PATCH`; prereleases may use a suffix such as `v1.2.0-rc.1`.

## Version resolution

A Protonman binary resolves its current version in this order:

1. build-time `-ldflags -X` injection
2. Go module build metadata from `debug.ReadBuildInfo`
3. `dev` when neither source contains a release version

`buildinfo.Version()` removes the leading `v`, so a binary built from tag
`v1.2.3` reports `Protonman 1.2.3` through `protonman --version`.

Local `make build` and `make dev` derive their injected version from:

```sh
git describe --tags --always --dirty --match 'v[0-9]*'
```

Override it explicitly when needed:

```sh
make build VERSION=v1.2.3
./bin/protonman --version
```

## Publishing a GitHub Release

Push a version tag after the release commit is on the remote:

```sh
git tag -a v1.2.3 -m "Protonman v1.2.3"
git push origin v1.2.3
```

`.github/workflows/release.yml` then:

- validates the version tag
- runs `go test ./...` and the offline installer integration tests
- builds Linux amd64 and macOS arm64
- injects the exact Git tag into every binary
- smoke-tests the native Linux amd64 binary with `--version`
- packages `protonman_<version>_<os>_<arch>` archives
- generates SHA-256 checksums
- creates the GitHub Release with generated release notes
- installs the published release through `install.sh` and verifies its embedded version

Prerelease tags such as `v1.2.3-rc.1` create a GitHub prerelease. Re-running
the workflow is safe: existing release assets are uploaded with `--clobber`.

Release assets use these names:

```text
protonman_1.2.3_linux_amd64.tar.gz
protonman_1.2.3_darwin_arm64.tar.gz
checksums.txt
install.sh
```

The executable inside each archive is `protonman`.

GitHub's latest-release API is intentionally not used to determine the current
binary version. It may be used separately for update checks, because the latest
published release and the binary currently running are different facts.

A versioned Go install also works without explicit linker flags because the existing
`debug.ReadBuildInfo` fallback reads the module version:

```sh
go install github.com/phongsathornpt/protonman/cmd/protonman@v1.2.3
protonman --version
```
