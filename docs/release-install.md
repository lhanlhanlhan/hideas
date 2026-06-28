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

## Server Deployment

The repository ships a `Dockerfile` and an interactive helper for deploying the
server side via Docker Compose:

```bash
./scripts/deploy.sh             # writes to ./deploy/
./scripts/deploy.sh /opt/hideas # custom deployment directory
```

The script asks for the public base URL, base path, host/port, SSO issuer,
SSO `client_id` / `client_secret`, scopes, and whether to generate a static
bearer token. It then writes `<dir>/config` and `<dir>/docker-compose.yml`,
builds the Docker image (`hideas:latest` by default; override with
`HIDEAS_IMAGE`), and optionally starts the stack with `docker compose up -d`.

The `Dockerfile` consumes the same prebuilt release tarball that
`scripts/install.sh` does. It does not build hideas from source. When prompted
by `deploy.sh` you can pick the release tag (default `latest`) and repo
(default `lhanlhanlhan/hideas`). The runtime base is `debian:bookworm-slim`
because the release binary is built with cgo against glibc.

Generated artifacts:

```text
<dir>/config            # TOML config consumed by `hideas serve`
<dir>/docker-compose.yml
<dir>/data/             # persistent SQLite volume
```

The script prints the exact `redirect_url` you must register with your SSO
administrator. Re-running the script overwrites the config file with new
answers (the `data/` directory is preserved).
