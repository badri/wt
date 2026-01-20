# Hub Commands

Hub commands are run from your orchestration session to manage workers and view status.

## Session Management

### `wt` / `wt list`

List all active worker sessions.

```bash
wt
wt list
```

Output:
```
┌─ Active Sessions ────────────────────────────────────────┐
│ 🟢 toast    myproject-abc   Working   Add auth flow     │
│ 🟡 shadow   myproject-def   Idle      Fix login bug     │
│ 🔴 obsidian myproject-ghi   Error     Update API        │
└──────────────────────────────────────────────────────────┘
```

### `wt new <bead-id>`

Create a new worker session for a bead.

```bash
wt new myproject-abc123
```

**What it does:**

1. Creates git worktree in `~/worktrees/<name>/`
2. Creates branch from bead ID
3. Starts tmux session
4. Launches Claude Code
5. Marks bead as `in_progress`

**Options:**

| Flag | Description |
|------|-------------|
| `--name` | Override session name |
| `--no-attach` | Create without attaching |

### `wt <name>`

Switch to a session by name or bead ID.

```bash
wt toast
wt myproject-abc123
```

### `wt pick`

Interactive session picker.

```bash
wt pick
```

Uses fzf if available, otherwise shows numbered prompt.

### `wt watch`

Live TUI dashboard showing all session statuses.

```bash
wt watch
```

Updates in real-time as sessions change state.

### `wt kill <name>`

Kill a session without closing the bead.

```bash
wt kill toast
```

**What it does:**

- Terminates tmux session
- Removes worktree
- Does NOT update bead status
- Use for abandoned/stuck sessions

### `wt close <name>`

Complete work and clean up session.

```bash
wt close toast
```

**What it does:**

1. Commits any uncommitted changes
2. Pushes branch
3. Creates PR (if configured)
4. Updates bead status
5. Removes worktree and tmux session

---

## Hub Session

### `wt hub`

Create or attach to a dedicated hub session.

```bash
wt hub
```

The hub is a special tmux session for orchestration.

**Options:**

| Flag | Description |
|------|-------------|
| `--watch` | Attach and add watch pane |
| `--detach` | Detach from hub |
| `--kill` | Terminate hub session |

---

## Bead Management

### `wt ready [project]`

Show beads ready to work on (no blockers).

```bash
wt ready
wt ready myproject
```

### `wt create <project> <title>`

Create a new bead.

```bash
wt create myproject "Add user authentication"
```

### `wt beads <project>`

List all beads for a project.

```bash
wt beads myproject
```

---

## Project Management

### `wt projects`

List registered projects.

```bash
wt projects
```

### `wt project add <name> <path>`

Register a new project.

```bash
wt project add myproject ~/code/myproject
```

### `wt project config <name>`

Edit project configuration.

```bash
wt project config myproject
```

Opens config in `$EDITOR`.

### `wt project remove <name>`

Unregister a project.

```bash
wt project remove myproject
```

---

## Auto Mode

### `wt auto`

Autonomous batch processing of ready beads.

```bash
wt auto
wt auto --project=myproject
wt auto --max=5
```

**Options:**

| Flag | Description |
|------|-------------|
| `--project` | Filter to specific project |
| `--max` | Maximum sessions to spawn |
| `--merge-mode` | Override merge mode |
| `--timeout` | Timeout per session |
| `--dry-run` | Preview without executing |

### `wt auto --check`

Check auto mode status.

```bash
wt auto --check
```

### `wt auto --stop`

Stop auto processing.

```bash
wt auto --stop
```

---

## Handoff

### `wt handoff`

Hand off hub session to a fresh Claude instance.

```bash
wt handoff
wt handoff -m "Continue with priority fixes"
wt handoff -c  # Auto-collect state
```

**Options:**

| Flag | Description |
|------|-------------|
| `-m` | Include message in handoff |
| `-c` | Auto-collect state (sessions, ready beads) |
| `--dry-run` | Preview what would be collected |

---

## Past Sessions

### `wt seance`

List past sessions.

```bash
wt seance
```

### `wt seance <name>`

Resume a past Claude session.

```bash
wt seance toast
```

### `wt seance <name> -p "prompt"`

One-shot query to a past session.

```bash
wt seance toast -p "Where did you put the nginx config?"
```
