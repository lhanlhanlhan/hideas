---
name: hideas
description: Use when an agent should operate Hideas as a client through the hideas CLI to record, retrieve, inspect, connect, summarize, or check status for personal memory using concepts such as Entity, Trace, Relation, Type, and profile traces.
---

# Hideas

Hideas is a personal cognitive system for managing memory traces, stable entities, and relations between them.

Use this skill when the task involves:

- Recording a user's memory, note, event, thought, fact, quote, or reflection into Hideas.
- Searching or retrieving prior memory from Hideas.
- Inspecting an Entity, Trace, Relation, or Type.
- Inspecting version or build-time information for the local binary or a remote server.
- Inspecting the current Hideas mode, server prefix, or login state.
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

Hideas treats the configured default mode as state. After a successful remote login, the default mode becomes `remote-client` with the logged-in server prefix unless the user overrides it.

## How Agents Should Use Hideas

Prefer the smallest useful operation:

1. Use `hideas add` for a new memory trace.
2. Use `hideas search` before assuming a memory does not exist.
3. Use `hideas version` to inspect the local binary version, build time, or the remote server version in remote client mode.
4. Use `hideas status` to inspect the current mode, server prefix, and login state.
5. Use `hideas show` to inspect context before editing or linking.
6. Use `hideas link` only for meaningful relationships.
7. Use `hideas profile set` for broad impressions about an entity.
8. In remote mode, check `hideas auth status --server URL` before assuming the client is authenticated.
9. If not authenticated, use `hideas login --server URL --identity /path/to/key`.

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
