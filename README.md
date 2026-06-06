# Hideas

Hideas is a personal cognitive system for managing memory traces, stable entities, and the relations between them.

It is currently built for personal use. The goal is not to become a large knowledge-management platform, but to provide a small, portable, inspectable system for recording and retrieving personal memory.

Hideas provides:

- A `hideas` CLI for local and remote access
- A SQLite-backed personal memory database
- A `hideas serve` mode that exposes standard HTTP APIs
- A remote client mode where the CLI talks to a Hideas HTTP server

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

Initialize the local database:

```bash
hideas init
```

Add an entity:

```bash
hideas entity add "李雷" --type person
```

Add a trace:

```bash
hideas add "今天和李雷讨论了记忆系统，决定先用 SQLite。" --type thought --entity "李雷"
```

Search:

```bash
hideas search "SQLite"
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

## Operating Modes

### Local Mode

Local mode is the default. Commands read and write the local SQLite database directly.

```bash
hideas search "SQLite"
```

The default database path is an OS user data path:

```text
macOS:   ~/Library/Application Support/hideas/hideas.sqlite
Linux:   $XDG_DATA_HOME/hideas/hideas.sqlite
Linux fallback: ~/.local/share/hideas/hideas.sqlite
Windows: %APPDATA%\hideas\hideas.sqlite
```

You can override it:

```bash
hideas --db /path/to/hideas.sqlite search "SQLite"
HIDEAS_DB=/path/to/hideas.sqlite hideas search "SQLite"
```

### Server Mode

Run Hideas as an HTTP server:

```bash
hideas serve --host 127.0.0.1 --port 8765
```

Mount under a base path:

```bash
hideas serve --base-path /hideas/
```

With a static token:

```bash
hideas serve --host 0.0.0.0 --token "$HIDEAS_TOKEN"
```

With SSH login enabled:

```bash
hideas serve --host 0.0.0.0 --authorized-keys "$HOME/.hideas/authorized_keys"
```

### Remote Client Mode

Use a remote Hideas server as the data source:

```bash
hideas --server https://example.com/hideas/ search "SQLite"
```

Or through config:

```text
mode = "remote-client"
server = "https://example.com/hideas/"
identity = "~/.ssh/id_ed25519"
credentials = "~/.hideas/credentials.json"
```

Default config path:

```text
$HOME/.hideas/config
```

Login once with an SSH private key:

```bash
hideas login --server https://example.com/hideas/ --identity ~/.ssh/id_ed25519
hideas auth status --server https://example.com/hideas/
hideas status
hideas version
```

The issued bearer token is stored in a separate credentials file. The config file stores the default mode, remote server prefix, and credentials path. `hideas version` queries the remote server version when running in remote client mode.

## HTTP API

Hideas exposes a v1.0 HTTP API under:

```text
/api/v1
```

When using:

```bash
hideas serve --base-path /hideas/
```

the API prefix becomes:

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
