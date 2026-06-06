---
name: hideas
description: Use when an agent should operate Hideas as a client through the hideas CLI to record, retrieve, inspect, connect, or summarize personal memory using concepts such as Entity, Trace, Relation, Type, and profile traces.
---

# Hideas

Hideas is a personal cognitive system for managing memory traces, stable entities, and relations between them.

Use this skill when the task involves:

- Recording a user's memory, note, event, thought, fact, quote, or reflection into Hideas.
- Searching or retrieving prior memory from Hideas.
- Inspecting an Entity, Trace, Relation, or Type.
- Creating links between memories, entities, and relations.
- Updating an entity profile trace.
- Using the `hideas` CLI as a client.

## Core Concepts

- **Entity**: a stable anchor such as a person, project, book, webpage, place, concept, conversation, organization, or source.
- **Trace**: the smallest memory unit, such as an event, thought, fact, quote, decision, reflection, or profile summary.
- **Relation**: a typed connection between two nodes. Endpoints may be Entity, Trace, or Relation.
- **Type**: a lightweight dictionary entry that prevents uncontrolled free-form type strings.

Key relationships:

- A `Trace` can be `about` an `Entity`.
- An `Entity` can have a `profile_trace_id` pointing to a profile `Trace`.
- An `Entity` can be an `alias_of` another `Entity`.
- A `Trace` can `support` or `contradict` another `Trace` or `Relation`.
- A `Relation` can itself be referenced as evidence or context.

Entity names are not unique. Always use entity IDs for stable identity. When a name resolves to multiple entities, ask for or require an explicit ID.

## How Agents Should Use Hideas

Prefer the smallest useful operation:

1. Use `hideas add` for a new memory trace.
2. Use `hideas search` before assuming a memory does not exist.
3. Use `hideas show` to inspect context before editing or linking.
4. Use `hideas link` only for meaningful relationships.
5. Use `hideas profile set` for broad impressions about an entity.
6. In remote mode, check `hideas auth status --server URL` before assuming the client is authenticated.
7. If not authenticated, use `hideas login --server URL --identity /path/to/key`.

Do not over-model. Only create an Entity when it is likely to be searched, linked, or summarized again.

## CLI-Only Usage

Agents should operate Hideas through the `hideas` CLI. Do not access Hideas through lower-level interfaces from this skill.

For detailed client commands, config, and examples, read:

- `references/operations.md`

You may also run:

```bash
hideas --help
hideas COMMAND --help
```

if the installed CLI supports help for the command being used.
