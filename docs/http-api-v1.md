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

If the server is started with a token:

```bash
hideas serve --token TOKEN
```

clients must send:

```http
Authorization: Bearer TOKEN
```

If no token is configured, authentication is not required.

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

