# hideas CLI Design v1.0

`hideas` is the command-line interface for accessing a personal memory database.

The CLI is a thin client. All commands talk to a hideas server over HTTP. The CLI never opens the SQLite database directly.

## Operating Model

`hideas` has two operating modes:

1. **Client mode** (the only mode for the `hideas` binary's data commands): every command talks to a configured hideas server.
2. **Server mode** (`hideas serve`): the same binary can act as the HTTP server that exposes the SQLite-backed memory database.

The CLI requires a configured server URL before any data command can run. It refuses to start data commands if no server is configured or no token is available.

## Client Mode

In client mode, commands call the remote HTTP API.

Example:

```bash
hideas add "今天和李雷讨论了记忆系统，决定先用 SQLite。"
hideas search "SQLite"
hideas show trace 123
```

The server is resolved from:

1. `--server` CLI option
2. `HIDEAS_SERVER` environment variable
3. `server` in the configuration file

```bash
hideas --server https://example.com/hideas/ search "SQLite"
HIDEAS_SERVER=https://example.com/hideas/ hideas search "SQLite"
```

```toml
server = "https://example.com/hideas/"
```

When neither a server nor a token is available, data commands exit with an error such as `server is required` or `not logged in: run 'hideas login' first`.

All command output should remain consistent with the JSON shapes documented in the HTTP API spec.

## Server Mode

Server mode exposes the memory database through HTTP.

```bash
hideas serve
hideas serve --config /etc/hideas/config
```

Server configuration is read entirely from the configuration file (or `HIDEAS_SSO_*` environment variables for SSO). `hideas serve` only accepts the `--config` flag; this prevents secrets like `client_secret` from appearing on the command line.

Server configuration keys:

```toml
db = "/path/to/hideas.sqlite"
host = "127.0.0.1"
port = 8765
base_path = "/"
token = "..."   # optional static bearer token for CI / self-tests

[sso]
issuer        = "https://sso.example.com/oauth"
client_id     = "your_client_id"
client_secret = "your_client_secret"
redirect_url  = "https://hideas.example.com/api/v1/auth/callback"
scopes        = "openid profile email"   # optional
```

Defaults:

- `host = "127.0.0.1"`
- `port = 8765`
- `base_path = "/"`
- `scopes = "openid profile email"`
- `db` defaults to an OS user data path (`~/Library/Application Support/hideas/hideas.sqlite` on macOS, `$XDG_DATA_HOME/hideas/hideas.sqlite` or `~/.local/share/hideas/hideas.sqlite` on Linux, `%APPDATA%\hideas\hideas.sqlite` on Windows).

The server validates `redirect_url` on startup: its path must equal `<base_path>/api/v1/auth/callback`. The server refuses to start when the SSO section is partially configured.

The server may also run with only `token` configured (no SSO). This is intended for CI/self-tests and emergency access. In that mode, clients must send `Authorization: Bearer <token>` and SSO endpoints are disabled.

## Configuration File

```bash
hideas --config /path/to/config search "SQLite"
```

If `--config` is not provided, the path is resolved from:

1. `HIDEAS_CONFIG`
2. `$HOME/.hideas/config`

Missing configuration files are ignored.

The v1.0 configuration format is TOML. All keys are optional:

```toml
# Client-side keys
server      = "https://example.com/hideas/"
token       = "..."
credentials = "~/.hideas/credentials.json"

# Server-side keys
db        = "/path/to/hideas.sqlite"
host      = "0.0.0.0"
port      = 8765
base_path = "/hideas/"

[sso]
issuer        = "..."
client_id     = "..."
client_secret = "..."
redirect_url  = "..."
scopes        = "openid profile email"
```

CLI precedence:

```text
CLI flag > environment variable > config file
```

Recognised client environment variables: `HIDEAS_SERVER`, `HIDEAS_TOKEN`, `HIDEAS_CONFIG`, `HIDEAS_CREDENTIALS`.

Recognised server environment variables: `HIDEAS_SSO_ISSUER`, `HIDEAS_SSO_CLIENT_ID`, `HIDEAS_SSO_CLIENT_SECRET`, `HIDEAS_SSO_REDIRECT_URL`, `HIDEAS_SSO_SCOPES`.

## Output Formats

The default output is human-readable text.

Machine-readable output is available through a format option:

```bash
hideas search "SQLite" --format json
```

Supported formats: `text`, `json`.

The same command returns the same JSON schema as documented in `docs/http-api-v1.md`.

`hideas --version` prints the local binary version and build time.

## Help

```bash
hideas --help
hideas help entity
hideas entity add --help
```

## Core Commands

### serve

Run the HTTP server. See [Server Mode](#server-mode).

```bash
hideas serve
hideas serve --config /etc/hideas/config
```

### login

Start an SSO login session against the configured server.

```bash
hideas login --server https://example.com/hideas/
hideas login --wait
hideas login --wait --timeout 10m
```

Behavior:

1. The CLI calls `POST /api/v1/auth/login/start`, prints the returned authorization URL, and writes the `pending_session_id` into `credentials.json`.
2. The user opens the URL in a browser and completes the SSO sign-in. The SSO redirects back to the hideas server's callback endpoint, which finalizes the login session.
3. Without `--wait`, the CLI exits after step 1. The next hideas command (or `hideas auth status`) polls the server once, picks up the issued token, and continues.
4. With `--wait`, the CLI polls in-place until the session resolves to `ready`, `expired`, or the configured timeout fires.

### logout

Remove the stored token for a remote server. If that server is also the configured default `server`, the `server` key in the config file is cleared.

```bash
hideas logout --server https://example.com/hideas/
```

### auth status

Verify that the current server has a stored, working token. If a pending session is in flight, this command opportunistically polls it.

```bash
hideas auth status --server https://example.com/hideas/
```

### status

Show the configured server and login state.

```bash
hideas status
```

### version

Show the current version and build time. When a server is configured, `hideas version` queries the server's version endpoint and falls back to the local binary version if the server is unreachable.

```bash
hideas --version
hideas version
```

### add

Create a new trace.

```bash
hideas add "今天和李雷讨论了记忆系统，决定先用 SQLite。"
```

Options:

```bash
--type TYPE
--at TIMESTAMP_OR_DATE
--entity NAME
--entity-id ID
```

Entity name resolution must handle ambiguity. If `--entity NAME` matches multiple entities, the command fails and asks for explicit `--entity-id`.

### search

Search traces and entities.

```bash
hideas search "SQLite"
```

Options:

```bash
--entity NAME
--entity-id ID
--type TYPE
--since TIMESTAMP_OR_DATE
--until TIMESTAMP_OR_DATE
--recent DURATION
--literal
--limit N
--format text|json
```

`--recent` is a CLI shortcut that expands to a time window ending at now. Supported units are `h`, `w`, and `y`, and the numeric part must be an integer. Example: `--recent 24h`.

By default, search keeps the full query as a literal phrase and also expands space-separated eligible keyword tokens. Tokens are eligible only when they contain at least one non-ASCII character and are at least two runes long, so pure English, pure numeric, and English-number tokens are not expanded. Use `--literal` to disable keyword expansion.

### show

Show an object and its immediate context.

```bash
hideas show entity 42
hideas show trace 123
hideas show relation 9
```

### trace update

Update trace timestamps.

```bash
hideas trace update 123 --happened-at 2026-04-19
hideas trace update 123 --created-at 1713484800000 --updated-at 1713484800000
```

At least one timestamp option is required. If `--updated-at` is omitted, updating `--happened-at` or `--created-at` refreshes `updated_at` to now.

### delete

Delete an object.

```bash
hideas delete relation 9
hideas delete trace 123 --cascade
hideas delete entity 42 --cascade
```

Default behavior is conservative:

- If the target is referenced by any relation, deletion is rejected.
- If a trace is used as an entity profile, deletion is rejected.
- The error tells the user to retry with `--cascade` when appropriate.

### link

Create a relation manually.

```bash
hideas link trace 123 entity 42 --type about
hideas link trace 123 trace 124 --type supports
hideas link entity 1 entity 2 --type alias_of
hideas link trace 55 relation 9 --type supports
```

Valid node kinds:

```text
entity
trace
relation
```

### entity

Manage entities.

```bash
hideas entity add "李雷" --type person
hideas entity list
hideas entity list --type person
hideas entity show 42
hideas entity rename 42 "Li Lei"
```

Entity names are allowed to repeat. Entity lists must include IDs and profile hints when available.

### profile

Manage an entity profile trace.

```bash
hideas profile show 42
hideas profile set 42 "李雷是一个务实的人，经常提醒我避免过度设计。"
```

`profile set` creates a trace of type `profile` and sets `entities.profile_trace_id`.

### type

Manage type dictionary entries.

```bash
hideas type list
hideas type add entity person
hideas type add trace thought
hideas type add relation about
```

### db

Inspect the remote database.

```bash
hideas db stats
hideas db check
```

`db check` validates polymorphic relation endpoints.

### export

Export user data.

```bash
hideas export --format json
hideas export --format markdown
```

## HTTP API v1

The HTTP API mirrors application operations. See `docs/http-api-v1.md` for the full surface.

Suggested base prefix:

```text
/api/v1
```

When `base_path = "/hideas/"`, the prefix becomes:

```text
/hideas/api/v1
```

## Response Envelope

Remote responses use a consistent JSON envelope:

```json
{
  "ok": true,
  "data": {},
  "error": null
}
```

Error example:

```json
{
  "ok": false,
  "data": null,
  "error": {
    "code": "ambiguous_entity",
    "message": "Entity name is ambiguous: 李雷",
    "details": {
      "candidates": []
    }
  }
}
```

## Authentication

The server uses two authentication modes, both delivered as `Authorization: Bearer <token>`:

1. **SSO** (default for normal use): clients log in through `hideas login`, which drives an OIDC Authorization Code flow against the configured SSO. After browser-side authorization, the server issues a hideas session token bound to the SSO subject. The client holds only the hideas token; it never sees the SSO access token.

2. **Static bearer token**: configured via `token` in the server config or `HIDEAS_TOKEN` on the client. Intended for CI/self-tests; not subject to SSO.

The client stores its issued token in `credentials.json` (mode `0600`).

If the server is exposed over a network, TLS should be handled by a reverse proxy or deployment environment.

## v1.0 Scope

Recommended MVP:

```text
hideas serve
hideas login
hideas logout
hideas auth status
hideas status
hideas add
hideas search
hideas show
hideas delete
hideas link
hideas entity add
hideas entity list
hideas profile set
hideas db stats
hideas db check
hideas export --format json
```

Defer:

```text
Semantic search
Automatic entity extraction
Automatic profile summarization
Entity merge
Multi-user permissions
Sync conflict resolution
```
