# Hideas Operations

This reference gives practical ways to use Hideas from an agent.

The `hideas` CLI is always a client. Before running data commands, ensure a server is configured and a token is available.

## Login

Check the current auth state:

```bash
hideas auth status
hideas status
```

Log in (non-blocking; the CLI prints an authorization URL and exits):

```bash
hideas login --server https://example.com/hideas/
```

Log in and block until the browser flow completes:

```bash
hideas login --wait --timeout 10m
```

After login, the issued token is stored in `~/.hideas/credentials.json` and the active server is written to `~/.hideas/config`.

Log out:

```bash
hideas logout
```

## CLI Basics

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
hideas search "Skill Q2 规划"
hideas search "Skill Q2 规划" --literal
hideas search "SQLite" --format json
```

Search output in text mode shows concise summaries, not full trace bodies. Search responses include `traces_has_more` and `entities_has_more` so clients can tell when the result set was truncated by `--limit`.

By default, search keeps the full query as a literal phrase and also expands eligible space-separated keyword tokens. Tokens are eligible only when they contain at least one non-ASCII character and are at least two runes long, so pure English, pure numeric, and English-number tokens are not expanded. Use `--literal` for exact phrase search only.

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
```

`hideas --version` prints the local binary version and build time. `hideas version` queries the configured server's version endpoint, falling back to the local binary version if the server is unreachable.

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

Client-side keys (TOML):

```toml
server      = "https://example.com/hideas/"
token       = "..."                       # optional, overrides credentials.json
credentials = "~/.hideas/credentials.json"
```

Precedence:

```text
CLI option > environment variable > config file
```

Environment variables: `HIDEAS_SERVER`, `HIDEAS_TOKEN`, `HIDEAS_CONFIG`, `HIDEAS_CREDENTIALS`.

Use an alternate config path:

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
