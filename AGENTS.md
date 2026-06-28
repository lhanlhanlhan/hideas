# Agent Working Rules

This repository is a personal cognitive system named Hideas. It provides a Go CLI that always operates as a client of a hideas server, a SQLite-backed local store, an HTTP server (`hideas serve`), and a standard HTTP API for clients that do not use the CLI.

Agents working in this repository must keep implementation, tests, and documentation aligned.

## Source Of Truth

The main contracts are:

- `docs/database-design-v1.md`: database and core model contract
- `docs/http-api-v1.md`: HTTP API contract
- `docs/cli-design-v1.md`: CLI behavior and operating modes
- `README.md`: user-facing project overview

Implementation must not silently drift away from these documents.

## HTTP API Rules

All HTTP server and remote-client behavior must match `docs/http-api-v1.md`.

This includes:

- Endpoint paths
- Base path behavior
- Authentication behavior (SSO Authorization Code flow and static bearer token)
- Request JSON shapes
- Response envelope shape
- JSON field names
- Error codes and error response shape
- Object shapes for Entity, Trace, Relation, Type, search results, stats, and check results

If an HTTP API change is intentional, update all of these in the same change:

- `internal/hideas/http.go`
- `docs/http-api-v1.md`
- Relevant tests in `internal/hideas/cli_test.go` or new test files
- README or CLI docs if user-facing behavior changes

## Database And Model Rules

Database schema and model constants must match `docs/database-design-v1.md`.

This includes:

- `Entity`, `Trace`, `Relation`, and `Type` concepts
- 64-bit integer IDs
- UTC epoch milliseconds for time values
- Node kind constants
- Type domain constants
- Nullable fields
- Entity name non-uniqueness
- `profile_trace_id` behavior
- Relation endpoint semantics
- Conservative delete and cascade semantics
- Index and foreign key policy

If a model or schema change is intentional, update all of these in the same change:

- `internal/hideas/models.go`
- `internal/hideas/sqlite.go`
- `docs/database-design-v1.md`
- `docs/http-api-v1.md` when JSON/API shapes are affected
- Tests covering migration, CRUD behavior, and API exposure

## CLI Rules

The CLI is a thin HTTP client. It does not open the local SQLite database. All data commands require a configured `server` and a valid bearer token, either issued through SSO login or supplied as a static token.

The same operation should produce consistent output whether the user is authenticated through SSO or through a static token.

## Configuration Rules

The CLI and the server share a single TOML configuration file (`~/.hideas/config` by default).

- Client-side keys: `server`, `token`, `credentials`.
- Server-side keys: `db`, `host`, `port`, `base_path`, `token`, and the `[sso]` section (`issuer`, `client_id`, `client_secret`, `redirect_url`, `scopes`).
- `client_secret` MUST NOT be exposed through CLI flags. `hideas serve` accepts only `--config` and reads everything else from the config file or `HIDEAS_SSO_*` env vars.
- The server validates `redirect_url` on startup; it must end with `<base_path>/api/v1/auth/callback`.

## Testing Rules

Before finishing code changes, run:

```bash
go test ./... -count=1
```

When editing `scripts/install.sh`, also run:

```bash
sh -n scripts/install.sh
```

Tests should cover:

- CLI client behavior against an in-memory HTTP server
- HTTP server behavior, including SSO `start`/`callback`/`poll` flow
- Static-token authentication
- Entity name ambiguity
- Relation endpoint validation
- Config resolution when changed
- Redirect URL validation when SSO is configured

## Release Rules

Release assets are built by `.github/workflows/release.yml`.

The installer in `scripts/install.sh` must stay compatible with the release asset names documented in `docs/release-install.md`.

If asset names or supported platforms change, update:

- `.github/workflows/release.yml`
- `scripts/install.sh`
- `docs/release-install.md`
- `README.md` if install instructions change

## Engineering Constraints

- Prefer small, explicit changes.
- Do not introduce a new framework unless it clearly reduces complexity.
- Keep the project portable and personal-use friendly.
- Do not make entity names unique.
- Do not replace documented integer constants with ad hoc strings in stored data.
- Do not change JSON field names without updating the HTTP API spec.
- Do not add undocumented API endpoints.
- Do not add database tables or columns without updating the database design document.
- Do not reintroduce CLI local-database mode. The CLI is HTTP-only.
- Do not put `client_secret` or other long-lived secrets behind CLI flags.

