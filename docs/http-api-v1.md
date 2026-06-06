# Hideas HTTP API v1.0

This document defines the v1.0 HTTP API exposed by `hideas serve`.

The API is intended to be stable enough for building Hideas clients without using the CLI. The CLI remote mode uses the same API.

## Base URL

Default API prefix:

```text
/api/v1
```

If the server is started with:

```bash
hideas serve --base-path /hideas/
```

then the API prefix becomes:

```text
/hideas/api/v1
```

For examples in this document, assume:

```text
https://example.com/hideas/api/v1
```

## Authentication

Hideas supports two authentication modes for remote access:

1. Static bearer token
2. SSH challenge login that issues a short-lived bearer token

If the server is started with a static token:

```bash
hideas serve --token TOKEN
```

clients must send:

```http
Authorization: Bearer TOKEN
```

If the server is started with authorized SSH public keys:

```bash
hideas serve --authorized-keys /path/to/authorized_keys
```

clients may obtain a bearer token through:

1. `POST /auth/challenge`
2. `POST /auth/login`

and then use:

```http
Authorization: Bearer TOKEN
```

If no token or authorized key file is configured, authentication is not required.

## Content Type

Request bodies should use JSON:

```http
Content-Type: application/json
```

Responses are JSON:

```http
Content-Type: application/json
```

## Response Envelope

Every API response uses the same envelope.

Success:

```json
{
  "ok": true,
  "data": {},
  "error": null
}
```

Error:

```json
{
  "ok": false,
  "data": null,
  "error": {
    "code": "not_found",
    "message": "not found"
  }
}
```

Known error codes:

```text
unauthorized
ambiguous_entity
delete_blocked
not_found
error
```

## Primitive Conventions

IDs are signed 64-bit integers.

Times are UTC Unix timestamps in milliseconds.

Node kinds use small integers:

```text
1 = entity
2 = trace
3 = relation
```

Type domains use small integers:

```text
1 = entity_type
2 = trace_type
3 = relation_type
```

Entity names are not unique. Clients must use entity IDs for stable identity.

## Object Shapes

### Type

```json
{
  "id": 1,
  "domain": 1,
  "name": "person",
  "created_at": 1710000000000,
  "updated_at": 1710000000000
}
```

### Entity

```json
{
  "id": 1,
  "type_id": 1,
  "type_name": "person",
  "profile_trace_id": 3,
  "profile": "李雷是一个务实的人。",
  "created_at": 1710000000000,
  "updated_at": 1710000000000,
  "name": "李雷"
}
```

Nullable fields may be omitted.

### Trace

```json
{
  "id": 1,
  "type_id": 5,
  "type_name": "thought",
  "happened_at": 1710000000000,
  "created_at": 1710000000000,
  "updated_at": 1710000000000,
  "content": "今天和李雷讨论了记忆系统。"
}
```

### Relation

```json
{
  "id": 1,
  "from_kind": 2,
  "from_id": 1,
  "to_kind": 1,
  "to_id": 1,
  "type_id": 7,
  "type_name": "about",
  "created_at": 1710000000000,
  "updated_at": 1710000000000
}
```

### ShowResult

```json
{
  "kind": "trace",
  "trace": {},
  "entities": [],
  "relations": []
}
```

Only the relevant root object is present:

```text
entity
trace
relation
```

## Health

### GET /health

Returns server health.

Response data:

```json
{
  "status": "ok"
}
```

Example:

```bash
curl -fsSL https://example.com/hideas/api/v1/health
```

## Authentication Endpoints

### POST /auth/challenge

Issues a one-time challenge for SSH login.

Request:

```json
{
  "client": "hideas-cli"
}
```

Response data:

```json
{
  "challenge_id": "7QF2xS2Wm9c9JY6R1G6d3s8r8Q6N4Y7A",
  "challenge": "1Yw4XgM4z3+f3P6jv5c+f6k7x7n0y+v8q8nL2jR5z3s=",
  "expires_at": 1710000000000
}
```

### POST /auth/login

Verifies an SSH signature over the challenge and issues a bearer token.

Request:

```json
{
  "challenge_id": "7QF2xS2Wm9c9JY6R1G6d3s8r8Q6N4Y7A",
  "public_key": "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI...",
  "signature": "BASE64_SSH_SIGNATURE"
}
```

Response data:

```json
{
  "token": "eyJhbGciOi...",
  "expires_at": 1710000000000
}
```

## Traces

### POST /traces

Creates a trace.

Request:

```json
{
  "Content": "今天和李雷讨论了记忆系统，决定先用 SQLite。",
  "TypeName": "thought",
  "Happened": 1710000000000,
  "EntityIDs": [1],
  "Entities": ["李雷"]
}
```

Fields:

```text
Content    required
TypeName   optional trace type name
Happened   optional UTC epoch milliseconds
EntityIDs  optional explicit entity IDs
Entities   optional entity names
```

If `Entities` contains a name that matches multiple entities, the request fails with `ambiguous_entity`.

Response data:

```json
{
  "id": 10,
  "type_name": "thought",
  "content": "今天和李雷讨论了记忆系统，决定先用 SQLite。"
}
```

### GET /traces/{id}

Shows a trace and its immediate context.

Response data:

```json
{
  "kind": "trace",
  "trace": {},
  "entities": [],
  "relations": []
}
```

### DELETE /traces/{id}

Deletes a trace.

By default, deletion is rejected if the trace is referenced by any relation or used as an entity profile.

Query parameters:

```text
cascade  optional boolean, true/false
```

With `cascade=true`, related relations are deleted recursively and any entity `profile_trace_id` pointing to the trace is cleared. Connected entities are not deleted.

Response data:

```json
{
  "kind": "trace",
  "id": 12,
  "cascade": true,
  "relations_deleted": 2,
  "profiles_cleared": 1,
  "deleted_relation_ids": [4, 5]
}
```

## Search

### GET /search

Searches traces and entities.

Query parameters:

```text
q          optional text query
entity     optional entity name
entity_id  optional entity ID
type       optional trace type name
since      optional UTC epoch milliseconds
until      optional UTC epoch milliseconds
limit      optional result limit
```

Example:

```bash
curl -fsSL "https://example.com/hideas/api/v1/search?q=SQLite&limit=20"
```

Response data:

```json
{
  "traces": [],
  "entities": []
}
```

## Entities

### POST /entities

Creates an entity.

Request:

```json
{
  "Name": "李雷",
  "Type": "person"
}
```

Fields:

```text
Name  required
Type  optional entity type name
```

Entity names are allowed to repeat.

Response data:

```json
{
  "id": 1,
  "name": "李雷",
  "type_name": "person"
}
```

### GET /entities

Lists entities.

Query parameters:

```text
type  optional entity type name
name  optional exact entity name
```

If `name` is provided, the endpoint returns matching candidates for name resolution.

Response data:

```json
[
  {
    "id": 1,
    "name": "李雷",
    "type_name": "person"
  }
]
```

### GET /entities/{id}

Shows an entity and its immediate context.

Response data:

```json
{
  "kind": "entity",
  "entity": {},
  "traces": [],
  "relations": []
}
```

### PATCH /entities/{id}

Renames an entity.

Request:

```json
{
  "Name": "Li Lei"
}
```

Response data:

```json
{
  "id": 1,
  "name": "Li Lei"
}
```

### DELETE /entities/{id}

Deletes an entity.

By default, deletion is rejected if the entity is referenced by any relation.

Query parameters:

```text
cascade  optional boolean, true/false
```

With `cascade=true`, related relations are deleted recursively. Connected traces or entities are not deleted.

Response data:

```json
{
  "kind": "entity",
  "id": 1,
  "cascade": true,
  "relations_deleted": 3,
  "profiles_cleared": 0,
  "deleted_relation_ids": [1, 2, 3]
}
```

## Profiles

### PUT /profiles/{entity_id}

Creates a profile trace and sets it as the entity profile.

Request:

```json
{
  "Content": "李雷是一个务实的人，经常提醒我避免过度设计。"
}
```

Response data:

```json
{
  "id": 12,
  "type_name": "profile",
  "content": "李雷是一个务实的人，经常提醒我避免过度设计。"
}
```

### GET /profiles/{entity_id}

Returns the current profile trace for an entity.

Response data:

```json
{
  "id": 12,
  "type_name": "profile",
  "content": "李雷是一个务实的人，经常提醒我避免过度设计。"
}
```

## Relations

### POST /relations

Creates a relation.

Request:

```json
{
  "FromKind": "trace",
  "FromID": 1,
  "ToKind": "entity",
  "ToID": 1,
  "Type": "about"
}
```

Valid kind strings:

```text
entity
trace
relation
```

Response data:

```json
{
  "id": 1,
  "from_kind": 2,
  "from_id": 1,
  "to_kind": 1,
  "to_id": 1,
  "type_name": "about"
}
```

### GET /relations/{id}

Shows a relation and its immediate context.

Response data:

```json
{
  "kind": "relation",
  "relation": {},
  "relations": []
}
```

### DELETE /relations/{id}

Deletes a relation.

By default, deletion is rejected if another relation references this relation.

Query parameters:

```text
cascade  optional boolean, true/false
```

With `cascade=true`, relations that reference this relation are deleted recursively.

Response data:

```json
{
  "kind": "relation",
  "id": 1,
  "cascade": true,
  "relations_deleted": 2,
  "profiles_cleared": 0,
  "deleted_relation_ids": [1, 7]
}
```

## Types

### GET /types

Lists all type dictionary entries.

Response data:

```json
[
  {
    "id": 1,
    "domain": 1,
    "name": "person"
  }
]
```

### POST /types

Creates or returns a type dictionary entry.

Request:

```json
{
  "Domain": "entity",
  "Name": "person"
}
```

Valid domain strings:

```text
entity
trace
relation
```

Response data:

```json
{
  "id": 1,
  "domain": 1,
  "name": "person"
}
```

## Database

### GET /db/stats

Returns object counts.

Response data:

```json
{
  "entities": 1,
  "traces": 2,
  "relations": 3,
  "types": 21
}
```

### GET /db/check

Validates relation endpoints.

Response data:

```json
{
  "ok": true,
  "errors": []
}
```

## Export

### GET /export

Exports user data.

Query parameters:

```text
format  optional, json or markdown
```

Response data:

```json
{
  "format": "json",
  "content": "{...}"
}
```

## Client Notes

Clients should:

- Use IDs, not names, as stable references.
- Treat entity name lookup as candidate resolution.
- Support `ambiguous_entity` by asking the user to choose an entity ID.
- Preserve unknown fields in responses where possible.
- Send UTC epoch milliseconds for time fields.
