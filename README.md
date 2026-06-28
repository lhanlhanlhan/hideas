# Hideas

Hideas is a personal cognitive system for managing memory traces, stable entities, and the relations between them.

It is currently built for personal use. The goal is not to become a large knowledge-management platform, but to provide a small, portable, inspectable system for recording and retrieving personal memory.

Hideas is split into:

- A `hideas serve` HTTP server backed by a SQLite-backed personal memory database.
- A `hideas` CLI that always operates as a client of a hideas server. The CLI does not read SQLite directly.

The HTTP API is intentionally documented as a first-class interface. You can build a Hideas client without using the CLI.

## Core Model

Hideas uses four core concepts:

- `Entity`: a stable anchor, such as a person, project, book, webpage, place, concept, or conversation
- `Trace`: the smallest memory unit, such as an event, thought, fact, quote, reflection, or profile
- `Relation`: a typed connection between entities, traces, or other relations
- `Type`: a lightweight dictionary entry used to avoid uncontrolled free-form type strings

See [docs/database-design-v1.md](docs/database-design-v1.md) for the v1.0 database design.

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

See [docs/release-install.md](docs/release-install.md) for release assets and installation details.

## Build From Source

```bash
go build -o hideas ./cmd/hideas
```

The project uses `github.com/mattn/go-sqlite3`, so cgo is required.

## Quick Start

The CLI is a client. To use it, point it at a hideas server and log in.

```bash
hideas login --server https://example.com/hideas/
```

`hideas login` prints an authorization URL. Open it in your browser and complete the SSO sign-in. Then run any hideas command (or `hideas auth status`) and the CLI will pick up the issued token automatically.

For an interactive script, use `--wait` to block until the browser flow completes:

```bash
hideas login --wait
```

After login:

```bash
hideas entity add "李雷" --type person
hideas add "今天和李雷讨论了记忆系统，决定先用 SQLite。" --type thought --entity "李雷"
hideas search "SQLite"
hideas search "Skill Q2 规划"
hideas search "Skill Q2 规划" --literal
hideas status
hideas version
```

Show an object:

```bash
hideas show trace 1
hideas show entity 1
```

Update trace timestamps:

```bash
hideas trace update 1 --happened-at 2026-04-19
```

Delete an object:

```bash
hideas delete relation 9
hideas delete trace 1 --cascade
hideas delete entity 1 --cascade
```

Get command help:

```bash
hideas --help
hideas help entity
hideas entity add --help
hideas --version
```

## Server Mode

Run a Hideas server. Server configuration lives entirely in the config file or in `HIDEAS_SSO_*` environment variables; `hideas serve` only accepts `--config`.

```bash
hideas serve
hideas serve --config /etc/hideas/config
```

A minimal server config:

```toml
db = "/var/lib/hideas/hideas.sqlite"
host = "0.0.0.0"
port = 8765
base_path = "/hideas/"

# Optional: static bearer token for CI / self-tests.
token = "..."

[sso]
issuer       = "https://sso.example.com/oauth"
client_id    = "your_client_id"
client_secret = "your_client_secret"
redirect_url = "https://hideas.example.com/hideas/api/v1/auth/callback"
# scopes is optional; defaults to "openid profile email".
```

`redirect_url` must end with `<base_path>/api/v1/auth/callback`. The server validates this on startup. Register the same URL with your SSO administrator.

## Client Configuration

The CLI also reads `~/.hideas/config`. After a successful `hideas login`, the CLI writes the server URL there automatically.

```toml
server      = "https://example.com/hideas/"
credentials = "~/.hideas/credentials.json"
```

`credentials.json` is created with `0600` permissions and stores the issued session token and any pending login session.

CLI configuration precedence:

```text
CLI flag > environment variable > config file
```

Environment variables: `HIDEAS_SERVER`, `HIDEAS_TOKEN`, `HIDEAS_CONFIG`, `HIDEAS_CREDENTIALS`.

## HTTP API

Hideas exposes a v1.0 HTTP API under:

```text
/api/v1
```

When using `base_path = "/hideas/"` the prefix becomes:

```text
/hideas/api/v1
```

See [docs/http-api-v1.md](docs/http-api-v1.md) for the full HTTP API specification.

## Documentation

- [CLI design v1.0](docs/cli-design-v1.md)
- [Database design v1.0](docs/database-design-v1.md)
- [HTTP API v1.0](docs/http-api-v1.md)
- [Release and install](docs/release-install.md)

## Status

Hideas is early and currently optimized for personal use.
