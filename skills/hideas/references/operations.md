# Hideas Operations

This reference gives practical ways to use Hideas from an agent.

## CLI Basics

Initialize the local database:

```bash
hideas init
```

Add a trace:

```bash
hideas add "今天和李雷讨论了记忆系统，决定先用 SQLite。" --type thought
```

Add a trace linked to an existing entity ID:

```bash
hideas add "今天和李雷讨论了记忆系统。" --type thought --entity-id 42
```

Add a trace linked by entity name:

```bash
hideas add "今天和李雷讨论了记忆系统。" --entity "李雷"
```

If the name is ambiguous, retry with `--entity-id`.

Search:

```bash
hideas search "SQLite"
hideas search "SQLite" --entity-id 42
hideas search --type thought --since 1710000000000
hideas search "SQLite" --format json
```

Show context:

```bash
hideas show trace 123
hideas show entity 42
hideas show relation 9
```

Create entities:

```bash
hideas entity add "李雷" --type person
hideas entity list
hideas entity list --type person
hideas entity rename 42 "Li Lei"
```

Set or show profile:

```bash
hideas profile set 42 "李雷是一个务实的人，经常提醒我避免过度设计。"
hideas profile show 42
```

Create relation:

```bash
hideas link trace 123 entity 42 --type about
hideas link trace 123 trace 124 --type supports
hideas link entity 1 entity 2 --type alias_of
hideas link trace 55 relation 9 --type supports
```

Inspect database:

```bash
hideas db path
hideas db stats
hideas db check
```

Export:

```bash
hideas export --format json
hideas export --format markdown
```

## Config

Default config path:

```text
$HOME/.hideas/config
```

Supported keys:

```text
db = "/path/to/hideas.sqlite"
```

Precedence:

```text
CLI option > environment variable > config file > built-in default
```

Config file example:

```bash
hideas --config /path/to/config search "SQLite"
```

## Agent Heuristics

Use Hideas when the user asks to remember, recall, connect, summarize, or inspect personal knowledge.

Do not store every incidental noun as an Entity. Create entities for stable anchors that are likely to recur.

Prefer short traces. If a source is long, create or link a source entity and store smaller traces derived from it.

Use profile traces for broad impressions, not for every fact.

Use relations when they preserve meaningful context:

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
