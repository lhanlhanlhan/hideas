# Release And Install

`hideas` publishes prebuilt binaries through GitHub Releases.

## Release Targets

The release workflow builds these assets:

```text
hideas-darwin-arm64.tar.gz
hideas-linux-amd64.tar.gz
hideas-linux-arm64.tar.gz
SHA256SUMS
```

The project uses `github.com/mattn/go-sqlite3`, so release builds run with cgo enabled.

## Create A Release

Push a version tag:

```bash
git tag v0.2
git push origin v0.2
```

The GitHub Action will build the binaries and upload them to the matching GitHub Release.

## Install

Latest release:

```bash
curl -fsSL https://raw.githubusercontent.com/lhanlhanlhan/hideas/main/scripts/install.sh | sh
```

Specific version:

```bash
curl -fsSL https://raw.githubusercontent.com/lhanlhanlhan/hideas/main/scripts/install.sh | HIDEAS_VERSION=v0.1 sh
```

Custom install directory:

```bash
curl -fsSL https://raw.githubusercontent.com/lhanlhanlhan/hideas/main/scripts/install.sh | HIDEAS_INSTALL_DIR="$HOME/.local/bin" sh
```

The installer detects the local OS and architecture, downloads the matching release asset, verifies `SHA256SUMS` when available, and installs the `hideas` binary.

