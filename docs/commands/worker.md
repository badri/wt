# Worker Commands

Worker commands are run from inside a wt session (the worker's tmux environment).

## Session Info

### `wt status`

Show current session information.

```bash
wt status
```

Output:
```
Session: toast
Bead: myproject-abc123
Title: Add user authentication
Worktree: ~/worktrees/toast
Branch: myproject-abc123
Port offset: 1
Status: working
Created: 2026-01-19 08:30
Last activity: 2026-01-19 10:45
```

---

## Completing Work

### `wt done`

Mark work complete, commit, push, and create PR.

```bash
wt done
```

**What it does:**

1. Stages all changes
2. Commits with descriptive message
3. Pushes branch to remote
4. Creates PR (based on merge mode)
5. Updates bead status to `awaiting_review`

**Options:**

| Flag | Description |
|------|-------------|
| `--merge-mode` | Override project merge mode |
| `--no-pr` | Skip PR creation |
| `-m` | Custom commit message |

### `wt close`

Same as `wt done` plus cleanup.

```bash
wt close
```

**What it does:**

1. Everything `wt done` does
2. Removes worktree
3. Terminates tmux session

### `wt abandon`

Discard uncommitted changes and close session.

```bash
wt abandon
```

!!! warning
    This discards all uncommitted work. Use with caution.

**What it does:**

1. Resets all changes (`git reset --hard`)
2. Removes worktree
3. Terminates tmux session
4. Does NOT update bead status

---

## Status Signaling

### `wt signal <status> [message]`

Update session status for the hub to see.

```bash
wt signal ready "Implementation complete, tests passing"
wt signal blocked "Need API credentials"
wt signal error "Build failing on line 42"
wt signal working "Refactoring auth module"
wt signal idle
```

**Statuses:**

| Status | Meaning |
|--------|---------|
| `working` | Actively making changes |
| `idle` | Waiting, not working |
| `ready` | Work complete, ready for review |
| `blocked` | Cannot proceed, needs help |
| `error` | Something went wrong |

The message is optional but helpful for context:

```bash
wt signal blocked "Waiting for database schema from backend team"
```

---

## Environment

Inside a worker session, these environment variables are available:

| Variable | Description | Example |
|----------|-------------|---------|
| `BEADS_DIR` | Path to main repo's beads | `/Users/you/myproject/.beads` |
| `PORT_OFFSET` | Port offset for test isolation | `1` |

### Using BEADS_DIR

All `bd` commands automatically use the main repo's beads:

```bash
# These work from the worktree
bd list
bd show myproject-abc123
bd update myproject-abc123 --status=in_progress
```

### Using PORT_OFFSET

Configure your services to use the offset:

```bash
# Start test database
docker run -d -p ${PORT_OFFSET}5432:5432 postgres

# Or in your .env
DATABASE_URL=postgres://localhost:${PORT_OFFSET}5432/mydb
```

---

## Git Operations

The worktree is a full git checkout. Normal git commands work:

```bash
git status
git diff
git log
git commit -m "message"
git push
```

However, prefer using `wt done` for completing work as it handles:

- Proper commit message formatting
- Branch pushing
- PR creation
- Bead status updates
