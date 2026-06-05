# Personal Memory Database Design v1.0

This document defines the v1.0 database design for a lightweight personal memory system.

The system is intentionally small. It models memory as a graph of stable anchors, memory traces, and relations, while keeping storage portable and efficient in a single SQLite database file.

## Core Concepts

### Entity

An `Entity` is a stable anchor in memory.

Examples:

- A person
- A book
- A webpage
- A project
- A place
- A concept
- A conversation
- A file

Entities should be created only for things that are likely to be searched, linked, or summarized again.

Entity names are not unique. Two different entities may have the same name, such as two people named `李雷` or multiple meanings of `Apple`.

The entity ID is the stable identity. The name is a display and lookup field.

### Trace

A `Trace` is the smallest memory unit.

Examples:

- An event
- A thought
- A fact
- A quote
- A judgment
- A reflection
- A profile summary

Traces should stay relatively short and close to their original content.

### Relation

A `Relation` connects two nodes.

The endpoints of a relation may be:

- Entity
- Trace
- Relation

This allows the system to express not only direct memory links, but also evidence about links.

Example:

```text
Trace --about--> Entity
Entity --alias_of--> Entity
Trace --supports--> Relation
Relation --derived_from--> Trace
```

### Type

A `Type` is a lightweight dictionary entry.

Types are not hard-coded enums and should not become a complex ontology. They exist to avoid uncontrolled free-form strings while still allowing the system to evolve.

## Storage Choice

Use SQLite.

Reasons:

- Portable single-file storage
- No database server required
- Efficient enough for a personal database
- Good relational query support
- Built-in B-tree indexes
- FTS5 can be added for full-text search
- A vector extension such as sqlite-vec can be added later for semantic search

## ID Strategy

All primary keys are SQLite `INTEGER PRIMARY KEY` values.

In SQLite, `INTEGER PRIMARY KEY` aliases the rowid, which is a signed 64-bit integer. This is sufficient for a personal memory database and avoids the storage overhead of string IDs.

## Time Strategy

All time fields are stored as integers:

```text
UTC Unix timestamp in milliseconds
```

Examples:

```text
created_at
updated_at
happened_at
```

This is more compact and efficient than ISO datetime text while still being easy to convert at the application layer.

## Node Kinds

Relations use small integers to identify the kind of each endpoint:

```text
1 = entity
2 = trace
3 = relation
```

These values are internal storage constants, not user-facing types.

## Type Domains

The `types.domain` field identifies which kind of object a type belongs to:

```text
1 = entity_type
2 = trace_type
3 = relation_type
```

## Schema

```sql
CREATE TABLE types (
  id INTEGER PRIMARY KEY,
  domain INTEGER NOT NULL,
  name TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,

  UNIQUE(domain, name)
);
```

```sql
CREATE TABLE entities (
  id INTEGER PRIMARY KEY,
  type_id INTEGER NULL,
  profile_trace_id INTEGER NULL,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  name TEXT NOT NULL,

  FOREIGN KEY (type_id) REFERENCES types(id),
  FOREIGN KEY (profile_trace_id) REFERENCES traces(id)
);
```

```sql
CREATE TABLE traces (
  id INTEGER PRIMARY KEY,
  type_id INTEGER NULL,
  happened_at INTEGER NULL,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  content TEXT NOT NULL,

  FOREIGN KEY (type_id) REFERENCES types(id)
);
```

```sql
CREATE TABLE relations (
  id INTEGER PRIMARY KEY,
  from_kind INTEGER NOT NULL,
  from_id INTEGER NOT NULL,
  to_kind INTEGER NOT NULL,
  to_id INTEGER NOT NULL,
  type_id INTEGER NOT NULL,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,

  FOREIGN KEY (type_id) REFERENCES types(id)
);
```

## Foreign Key Policy

Use foreign keys for direct single-table references:

- `entities.type_id -> types.id`
- `entities.profile_trace_id -> traces.id`
- `traces.type_id -> types.id`
- `relations.type_id -> types.id`

Do not use foreign keys for relation endpoints:

- `relations.from_id`
- `relations.to_id`

Reason: relation endpoints are polymorphic. They may point to an entity, trace, or another relation depending on `from_kind` and `to_kind`. SQLite cannot express this cleanly with ordinary foreign keys.

Endpoint validity should be checked in application logic.

## Indexes

Create indexes only for stable query paths.

```sql
CREATE INDEX idx_entities_type_id
ON entities(type_id);
```

```sql
CREATE INDEX idx_entities_name
ON entities(name);
```

```sql
CREATE INDEX idx_entities_profile_trace_id
ON entities(profile_trace_id);
```

```sql
CREATE INDEX idx_traces_type_id
ON traces(type_id);
```

```sql
CREATE INDEX idx_traces_happened_at
ON traces(happened_at);
```

```sql
CREATE INDEX idx_relations_from_type
ON relations(from_kind, from_id, type_id);
```

```sql
CREATE INDEX idx_relations_to_type
ON relations(to_kind, to_id, type_id);
```

```sql
CREATE INDEX idx_relations_type_id
ON relations(type_id);
```

The unique constraint on `types(domain, name)` already creates the needed index for type lookup and deduplication.

Do not create a unique index on `entities.name`. Name collisions are valid and must be handled by the application or CLI layer.

## Entity Name Resolution

Applications should treat entity name lookup as candidate resolution:

- If a name matches exactly one entity, it may be used directly.
- If a name matches no entities, the application may create a new entity or ask for confirmation.
- If a name matches multiple entities, the application must report ambiguity and require an explicit entity ID.

All user-facing entity lists should include IDs.

Example:

```text
42  李雷  person  profile: 前同事，做后端
87  李雷  person  profile: 大学同学，做设计
```

The profile trace is the preferred human-readable disambiguation hint.

## Optional Full-Text Search

Full-text search should use FTS5 instead of a normal B-tree index on `traces.content`.

```sql
CREATE VIRTUAL TABLE traces_fts
USING fts5(content, content='traces', content_rowid='id');
```

The exact synchronization strategy for FTS can be decided when full-text search is implemented.

## Initial Type Seeds

These are suggested initial values, not hard-coded enums.

Entity types:

```text
person
work
project
concept
source
place
other
```

Trace types:

```text
event
thought
fact
quote
profile
other
```

Relation types:

```text
about
mentions
part_of
alias_of
derived_from
supports
contradicts
related_to
```

## Design Rules

- `Trace` is the smallest memory unit.
- `Entity` is a stable memory anchor.
- Entity names may repeat. Entity IDs are the source of identity.
- `Relation` is a traceable connection.
- `Type` is a controlled dictionary, not a full ontology.
- `entities.type_id` may be null.
- `traces.type_id` may be null.
- `relations.type_id` is required.
- `entities.profile_trace_id` is a direct field because it is a high-frequency access path.
- Keep entities sparse. Not every noun should become an entity.
- Keep traces short. Long source content should be represented as an entity or source trace and linked from smaller traces.
- Keep relations meaningful. Avoid extracting too many low-value relations from each trace.
