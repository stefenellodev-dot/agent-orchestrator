# OpenCode CLI Capabilities (verified)

Recorded during plan execution. The orchestrator adapter MUST only rely on
capabilities confirmed here. Unknown/unverified flags are treated as absent.

## Environment

| Item | Value |
|------|-------|
| `opencode --version` | `1.18.31` |
| Probe date | 2026-09-28 |
| Platform | darwin/arm64 |

## Verified flags (`opencode run --help`)

| Capability | Flag | Verified | Notes |
|------------|------|----------|-------|
| JSON output | `--format` (`default`\|`json`) | yes | "json (raw JSON events)" |
| Session id | `--session` (`-s`) | yes | string |
| Continue last | `--continue` (`-c`) | yes | boolean |
| Fork session | `--fork` | yes | requires `--continue` or `--session` |
| Agent select | `--agent` | yes | string |
| Working dir | `--dir` | yes | local path, or remote path when attaching |
| Attach to server | `--attach` | yes | e.g. `http://localhost:4096` |
| Auto-approve | `--auto` | yes | "dangerous"; NOT used by MVP by default |
| Model | `--model` (`-m`) | yes | `provider/model` |
| Attach file | `--file` (`-f`) | yes | array |
| Title | `--title` | yes | string |
| Password/user | `--password` / `--username` (`-p`/`-u`) | yes | basic auth for server |

## Verified agents (`opencode agent list`)

| Agent | Kind |
|-------|------|
| `build` | primary |
| `compaction` | primary |
| `plan` | primary |
| `summary` | primary |
| `title` | primary |
| `explore` | subagent |
| `general` | subagent |

## MVP adapter decisions

1. **Independent phases by default.** Correctness MUST NOT depend on session
   continuity or `--fork`. Session flags exist but are not load-bearing.
2. **Agent names** are configured explicitly per phase in orchestrator config.
   Verified real agent names: `plan`, `build`, `explore`, `general`.
   Default mapping below.
3. **JSON parsing is best-effort.** `--format json` exists, but the exact event
   schema is not guaranteed stable. The adapter captures raw stdout and parses
   defensively; if parsing fails it still returns exit code + raw output.
4. **Objective evidence is collected from git and process exits**, not from
   agent self-report: `git rev-parse HEAD`, `git diff`, captured exit codes.

## Default phase → agent mapping

| Phase | Agent |
|-------|-------|
| discovery | `explore` |
| decision | `plan` |
| implementation | `build` |
| validation | `build` (runs the validation commands) |
