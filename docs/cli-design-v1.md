# hideas CLI Design v1.0

`hideas` is the command-line interface for accessing a personal memory database.

The CLI should remain thin. It should provide stable input, reliable retrieval, clear display, manual correction, and optional HTTP access to the same memory operations.

## Operating Modes

`hideas` supports three operating modes:

1. Local mode
2. Server mode
3. Remote client mode

The same user-facing commands should produce the same output shape whether the data source is local SQLite or a remote `hideas serve` HTTP service.

## Local Mode

Local mode is the default mode.

In local mode, commands read and write a SQLite database directly.

Example:

```bash
hideas add "今天和李雷讨论了记忆系统，决定先用 SQLite。"
hideas search "SQLite"
hideas show trace 123
```

The database path should be resolved from:

1. Explicit CLI option
2. Environment variable
3. Configuration file
4. Default local path

Suggested option, environment variable, and configuration key:

```bash
hideas --db /path/to/hideas.sqlite search "SQLite"
HIDEAS_DB=/path/to/hideas.sqlite hideas search "SQLite"
```

```text
db = "/path/to/hideas.sqlite"
```

The default local database path should be an OS user data path, not the current working directory:

```text
macOS:   ~/Library/Application Support/hideas/hideas.sqlite
Linux:   $XDG_DATA_HOME/hideas/hideas.sqlite
Linux fallback: ~/.local/share/hideas/hideas.sqlite
Windows: %APPDATA%\hideas\hideas.sqlite
```

The CLI should create the parent directory when opening or initializing the local database.

## Server Mode

Server mode exposes the memory database through HTTP.

Example:

```bash
hideas serve --db /path/to/hideas.sqlite --host 127.0.0.1 --port 8765
```

Suggested defaults:

```text
host = 127.0.0.1
port = 8765
base_path = /
```

The server should use the database configured by `--db` or `HIDEAS_DB`.

The server should expose HTTP endpoints using a stable protocol. It should not change CLI semantics. It is a transport layer over the same application operations used by local mode.

### Base Path

The server should support a configurable base path so it can be mounted behind a reverse proxy.

Example:

```bash
hideas serve --base-path /hideas/
```

If mounted at:

```text
https://example.com/hideas/
```

then API endpoints should live under that prefix.

## Remote Client Mode

Remote client mode uses a `hideas serve` HTTP service as the data source.

Example:

```bash
hideas --server https://example.com/hideas/ search "SQLite"
```

Or:

```bash
HIDEAS_SERVER=https://example.com/hideas/ hideas search "SQLite"
```

Or through a configuration file:

```text
mode = "remote-client"
server = "https://example.com/hideas/"
```

Effective mode resolution:

1. `--mode`
2. `HIDEAS_MODE`
3. `mode` in the configuration file
4. Built-in default `local`

Resolution order:

1. `--server`
2. `HIDEAS_SERVER`
3. `server` in the configuration file
4. Local mode

When the effective mode is `remote-client`, the CLI should not open the local SQLite database. It should call the remote HTTP API instead.

All command output should remain consistent with local mode.

## Configuration File

The CLI should support an explicit configuration path:

```bash
hideas --config /path/to/config search "SQLite"
```

If `--config` is not provided, the path should resolve from:

1. `HIDEAS_CONFIG`
2. `$HOME/.hideas/config`

Missing configuration files should be ignored.

The v1.0 configuration format is a small key-value file:

```text
mode = "remote-client"
db = "/path/to/hideas.sqlite"
server = "https://example.com/hideas/"
identity = "~/.ssh/id_ed25519"
credentials = "~/.hideas/credentials.json"
authorized_keys = "~/.hideas/authorized_keys"
```

Supported keys:

```text
mode
db
server
token
identity
credentials
authorized_keys
```

Global configuration precedence:

```text
CLI option > environment variable > configuration file > built-in default
```

## Output Formats

The default output should be human-readable text.

Machine-readable output should be available through a format option:

```bash
hideas search "SQLite" --format json
```

Suggested formats:

```text
text
json
```

The same command should return the same JSON schema in local and remote client modes.

The CLI should also support `hideas --version` to print the local binary version and build time.

## Help

The CLI should provide built-in help at both the root and subcommand levels.

Examples:

```bash
hideas --help
hideas help entity
hideas entity add --help
```

## Core Commands

### init

Initialize a local SQLite database.

```bash
hideas init
hideas init --db /path/to/hideas.sqlite
```

Responsibilities:

- Create schema
- Create indexes
- Seed initial types
- Enable required SQLite pragmas

### serve

Run the HTTP server.

```bash
hideas serve
hideas serve --db /path/to/hideas.sqlite
hideas serve --host 0.0.0.0 --port 8765
hideas serve --base-path /hideas/
hideas serve --authorized-keys ~/.hideas/authorized_keys
```

Responsibilities:

- Open configured SQLite database
- Listen on configured host and port
- Expose the standard HTTP API
- Return consistent response envelopes

### version

Show the current version and build time.

```bash
hideas --version
hideas version
```

When the CLI is running in remote client mode, `hideas version` should query the server version endpoint and print the server's version and build time.

### status

Show the current operating mode, configured remote server prefix, and login state.

```bash
hideas status
```

### login

Authenticate to a remote server with an SSH private key, store the issued token in a credentials file, and switch the default mode to remote client mode.

```bash
hideas login --server https://example.com/hideas/ --identity ~/.ssh/id_ed25519
```

### auth status

Verify that the current remote server has a stored, working token.

```bash
hideas auth status --server https://example.com/hideas/
```

### logout

Remove the stored token for a remote server. If that server is the configured default server, the default mode returns to local mode.

```bash
hideas logout --server https://example.com/hideas/
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

Responsibilities:

- Create a trace
- Resolve or create referenced entities
- Create relations from the trace to entities when requested

Entity name resolution must handle ambiguity. If `--entity NAME` matches multiple entities, the command must fail and ask for explicit `--entity-id`.

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

First version search should support:

- Exact entity filtering
- Type filtering
- Time filtering
- Full-text search when FTS is available
- Space-separated keyword expansion with conservative token filtering
- Result limiting with a `has_more` signal when more matches exist than the requested limit
- Summary output in text mode instead of full trace bodies

### show

Show an object and its immediate context.

```bash
hideas show entity 42
hideas show trace 123
hideas show relation 9
```

Responsibilities:

- Display the object
- Display type information
- Display connected entities, traces, and relations
- Always include IDs in output

### trace update

Update trace timestamps.

```bash
hideas trace update 123 --happened-at 2026-04-19
hideas trace update 123 --created-at 1713484800000 --updated-at 1713484800000
```

Options:

```bash
--happened-at TIMESTAMP_OR_DATE
--created-at TIMESTAMP_OR_DATE
--updated-at TIMESTAMP_OR_DATE
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
- The error should tell the user to retry with `--cascade` when appropriate.

Cascade behavior:

- Deletes relations that reference the target.
- Recursively deletes relations that reference those deleted relations.
- Clears `entities.profile_trace_id` when deleting a profile trace.
- Does not delete connected entities or traces other than the requested target.

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

`profile set` should create a trace of type `profile` and set `entities.profile_trace_id`.

### type

Manage type dictionary entries.

```bash
hideas type list
hideas type add entity person
hideas type add trace thought
hideas type add relation about
```

The type system should remain small. The CLI should not encourage creating many near-duplicate types.

### db

Inspect and validate the local database.

```bash
hideas db path
hideas db stats
hideas db check
```

`db check` should validate polymorphic relation endpoints because those endpoints are not protected by ordinary foreign keys.

### export

Export user data.

```bash
hideas export --format json
hideas export --format markdown
```

Export is important because the memory database should not be locked into one tool.

## HTTP API v1

The HTTP API should mirror application operations, not terminal formatting.

Suggested base prefix:

```text
/api/v1
```

When `--base-path /hideas/` is used, the full prefix becomes:

```text
/hideas/api/v1
```

### Health

```http
GET /api/v1/health
```

### Traces

```http
POST /api/v1/traces
GET /api/v1/traces/{id}
PATCH /api/v1/traces/{id}
DELETE /api/v1/traces/{id}
GET /api/v1/search
```

### Entities

```http
POST /api/v1/entities
GET /api/v1/entities/{id}
GET /api/v1/entities
PATCH /api/v1/entities/{id}
DELETE /api/v1/entities/{id}
```

### Relations

```http
POST /api/v1/relations
GET /api/v1/relations/{id}
DELETE /api/v1/relations/{id}
```

### Types

```http
GET /api/v1/types
POST /api/v1/types
```

### Database

```http
GET /api/v1/db/stats
GET /api/v1/db/check
```

## Response Envelope

Remote responses should use a consistent JSON envelope:

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

The CLI should map these errors to the same human-readable messages that local mode would produce.

## Authentication

Authentication is not required for local-only use.

When binding to anything other than `127.0.0.1`, the server should support authentication.

Suggested configuration:

```bash
hideas serve --host 0.0.0.0 --token TOKEN
hideas serve --host 0.0.0.0 --authorized-keys ~/.hideas/authorized_keys
hideas login --server https://example.com/hideas/ --identity ~/.ssh/id_ed25519
```

Suggested HTTP header:

```http
Authorization: Bearer TOKEN
```

For SSH login, the client should:

1. Request a challenge from the server
2. Sign the challenge with the configured SSH private key
3. Exchange the signature for a bearer token
4. Store the token in a separate credentials file

The credentials file should be outside the config file and created with `0600` permissions.

If the server is exposed over a network, TLS should be handled by a reverse proxy or deployment environment.

## Data Source Abstraction

CLI commands should depend on a data source interface, not directly on SQLite or HTTP.

Conceptually:

```text
Command -> MemoryStore -> SQLiteStore
Command -> MemoryStore -> HttpStore
```

This keeps local mode and remote client mode behavior aligned.

## v1.0 Scope

Recommended MVP:

```text
hideas init
hideas serve
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
Advanced auth
Multi-user permissions
Sync conflict resolution
```
