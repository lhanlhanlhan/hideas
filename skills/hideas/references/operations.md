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
hideas search "SQLite" --recent 24h
hideas search "SQLite" --format json
```

Search output in text mode should show concise summaries, not full trace bodies. Search responses include `traces_has_more` and `entities_has_more` so clients can tell when the result set was truncated by `--limit`.

`--recent` is a CLI shortcut that expands to a time window ending at now. Supported units are `h`, `w`, and `y`, and the numeric part must be an integer, for example `24h`, `2w`, or `1y`. Do not combine `--recent` with `--since` or `--until`.

Search time filters use `happened_at` when present and fall back to `created_at`. For imported historical material, set `happened_at` to the real event or source date; otherwise recent searches may include old content that was imported recently.

Update trace timestamps:

```bash
hideas trace update 123 --happened-at 2026-04-19
hideas trace update 123 --created-at 1713484800000
hideas trace update 123 --updated-at 1713484800000
```

At least one timestamp option is required. If `--updated-at` is omitted, changing `--happened-at` or `--created-at` refreshes `updated_at` to now.

Version:

```bash
hideas --version
hideas version
hideas version --server https://example.com/hideas/
```

`hideas --version` prints the local binary version and build time. In remote client mode, `hideas version` queries the connected server's version endpoint.

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

Delete objects:

```bash
hideas delete relation 9
hideas delete trace 123 --cascade
hideas delete entity 42 --cascade
```

Deletion is conservative by default. If Hideas reports `delete blocked`, inspect the object with `hideas show` and retry with `--cascade` only when deleting related relations is intended.

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

## Remote Authentication

When using a remote Hideas server, prefer the CLI login flow instead of passing bearer tokens around manually.

Check whether the client is already authenticated:

```bash
hideas auth status --server https://example.com/hideas/
```

If not logged in, authenticate with an SSH private key:

```bash
hideas login --server https://example.com/hideas/ --identity ~/.ssh/id_ed25519
```

Log out and remove the stored token:

```bash
hideas logout --server https://example.com/hideas/
```

After login succeeds, normal CLI commands can use the remote server directly:

```bash
hideas status
hideas search "SQLite"
hideas add "新的记忆" --type thought
```

## Config

Default config path:

```text
$HOME/.hideas/config
```

Supported keys:

```text
mode = "remote-client"
db = "/path/to/hideas.sqlite"
server = "https://example.com/hideas/"
identity = "~/.ssh/id_ed25519"
credentials = "~/.hideas/credentials.json"
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

Prefer `Trace` by default. Create an `Entity` only when the item is likely to recur, be linked, or be summarized again.

When a name is ambiguous, require an explicit entity ID instead of guessing.
