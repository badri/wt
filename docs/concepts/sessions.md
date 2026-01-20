# Sessions

A **session** is the core unit of work in wt. It combines a tmux session, a git worktree, and a Claude Code instance into a single managed entity.

## Session Components

Each session consists of:

```
Session "toast"
├── Tmux session (persistent terminal)
├── Git worktree (isolated code copy)
├── Branch (created from bead ID)
├── Claude Code instance
├── Bead reference (the task being worked on)
└── Port offset (for isolated test services)
```

## Session Names

Sessions are assigned names from a **namepool** - a themed list of names that makes sessions easy to identify and remember:

- `toast`, `shadow`, `obsidian`, `quartz`, `jasper`
- Theme examples: kung-fu-panda, toy-story, ghibli, star-wars

Names are:

- Unique per project (no two active sessions share a name)
- Recycled when sessions close
- Easier to type than bead IDs

## Session States

| State | Description |
|-------|-------------|
| `working` | Claude is actively using tools |
| `idle` | Waiting at prompt for 5+ minutes |
| `ready` | Work complete, awaiting review |
| `blocked` | Cannot proceed, needs help |
| `error` | Something went wrong |

## Creating Sessions

```bash
# Create session for a bead
wt new myproject-abc123

# Output:
# Spawned session 'toast' for bead myproject-abc123
#   Worktree: ~/worktrees/toast
#   Branch: myproject-abc123
#   Port offset: 1
```

## Listing Sessions

```bash
# List all sessions
wt

# Output:
# ┌─ Active Sessions ────────────────────────────────────────┐
# │ 🟢 toast    myproject-abc   Working   Add auth flow     │
# │ 🟡 shadow   myproject-def   Idle      Fix login bug     │
# │ 🔴 obsidian myproject-ghi   Error     Update API        │
# └──────────────────────────────────────────────────────────┘
```

## Switching Sessions

```bash
# By name
wt toast

# Interactive picker
wt pick
```

## Session Status

From inside a session:

```bash
wt status

# Output:
# Session: toast
# Bead: myproject-abc123
# Worktree: ~/worktrees/toast
# Branch: myproject-abc123
# Port offset: 1
# Status: working
```

## Signaling Status

Workers can signal their status:

```bash
wt signal ready "Implementation complete, tests passing"
wt signal blocked "Need API credentials"
wt signal error "Build failing"
```

## Closing Sessions

```bash
# From hub: close by name
wt close toast

# From inside session
wt close

# Or complete work first
wt done    # Commits, pushes, creates PR
wt close   # Cleans up session
```

## Session Persistence

Sessions are stored in `~/.config/wt/sessions.json`:

```json
{
  "toast": {
    "bead": "myproject-abc123",
    "project": "myproject",
    "worktree": "/Users/you/worktrees/toast",
    "branch": "myproject-abc123",
    "port_offset": 1,
    "status": "working",
    "created_at": "2026-01-19T08:30:00Z",
    "last_activity": "2026-01-19T10:45:00Z"
  }
}
```
