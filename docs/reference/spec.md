# Technical Specification

This document provides the complete technical specification for wt, including architecture details, configuration formats, and internal behavior.

For user-focused documentation, see the [Hub Workflow](../guides/hub-workflow.md) guide.

---

## Overview

`wt` is a minimal agentic coding orchestrator built on:
- **Beads** for task tracking
- **Git worktrees** for isolation
- **Tmux** for session persistence
- **Claude** (or other agents) for execution

## Philosophy

**One bead = one session = one worktree.**

Each task (bead) gets its own isolated environment: a dedicated git worktree, a persistent tmux session, and an AI agent working on just that task. Sessions persist until you explicitly close them—no auto-compaction, no context loss, no handoff complexity.

### Hub-and-Spoke Model

The architecture separates orchestration from execution:

- **Hub**: Your control center for grooming beads, spawning workers, and monitoring progress. This is where you make decisions about what work to do next.
- **Workers**: Isolated sessions where AI agents execute tasks autonomously. Each worker focuses on exactly one bead.

This separation means you can run multiple agents in parallel without conflicts, while maintaining visibility into what each one is doing.

### Design Principles

1. **Explicit over automatic**: Sessions don't auto-close or auto-compact. You control the lifecycle.
2. **Isolation over sharing**: Each worker has its own worktree, branch, and optionally its own test environment.
3. **Visibility over opacity**: `wt watch` shows all sessions at a glance. Signals communicate status.
4. **Simplicity over features**: Start simple, extend only when needed. The core commands are just `new`, `switch`, `done`, `close`.

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                           HUB                                   │
│  (Your grooming session - regular Claude in any terminal)       │
│                                                                 │
│  - Groom beads: bd create, bd ready, bd list                    │
│  - Spawn workers: wt new <bead-id>                              │
│  - Monitor: wt, wt watch                                        │
│  - Switch: wt <session-name>                                    │
└─────────────────────────────────────────────────────────────────┘
         │
         │ wt new supabyoi-pks
         ▼
┌─────────────────────────────────────────────────────────────────┐
│  WORKER SESSION: toast                                          │
│                                                                 │
│  Name:     toast (from namepool)                                │
│  Bead:     supabyoi-pks                                         │
│  Worktree: ~/worktrees/toast/                                   │
│  Branch:   supabyoi-pks                                         │
│  Env:      BEADS_DIR=~/supabyoi/.beads                          │
│            PORT_OFFSET=1                                        │
│  Services: docker compose up -d (ports 15432, 13000)            │
│                                                                 │
│  Claude running, working on the bead...                         │
└─────────────────────────────────────────────────────────────────┘
         │
         │ wt new supabyoi-g4a
         ▼
┌─────────────────────────────────────────────────────────────────┐
│  WORKER SESSION: shadow                                         │
│                                                                 │
│  Name:     shadow                                               │
│  Bead:     supabyoi-g4a                                         │
│  Worktree: ~/worktrees/shadow/                                  │
│  Branch:   supabyoi-g4a                                         │
│  Env:      BEADS_DIR=~/supabyoi/.beads                          │
│            PORT_OFFSET=2                                        │
│  Services: docker compose up -d (ports 25432, 23000)            │
└─────────────────────────────────────────────────────────────────┘
```

---

## Directory Structure

```
~/.config/wt/
├── config.json              # Global configuration
├── namepool.txt             # Available session names
├── sessions.json            # Active session state
└── projects/
    ├── supabyoi.json        # Per-project config
    └── reddit-saas.json

~/worktrees/                 # All worktrees live here
├── toast/                   # Worktree for session "toast"
├── shadow/                  # Worktree for session "shadow"
└── obsidian/                # Worktree for session "obsidian"
```

---

## Configuration

### Global Config (`~/.config/wt/config.json`)

```json
{
  "worktree_root": "~/worktrees",
  "editor_cmd": "claude",
  "default_merge_mode": "pr-review",
  "idle_threshold_minutes": 5,
  "notify_on_idle": true,
  "watch_interval_seconds": 30
}
```

### Namepool (`~/.config/wt/namepool.txt`)

```
toast
shadow
obsidian
quartz
jasper
onyx
opal
topaz
marble
granite
amber
crystal
flint
slate
copper
bronze
silver
cobalt
iron
steel
```

### Project Config (`~/.config/wt/projects/<project>.json`)

```json
{
  "name": "supabyoi",
  "repo": "~/supabyoi",
  "default_branch": "main",
  "beads_prefix": "supabyoi",

  "merge_mode": "pr-review",
  "require_ci": true,
  "auto_merge_on_green": false,

  "test_env": {
    "setup": "docker compose up -d",
    "teardown": "docker compose down",
    "port_env": "PORT_OFFSET",
    "health_check": "curl -f http://localhost:${PORT_OFFSET}3000/health"
  },

  "hooks": {
    "on_create": [
      "npm install"
    ],
    "on_close": [
      "docker compose down"
    ]
  }
}
```

### Session State (`~/.config/wt/sessions.json`)

```json
{
  "toast": {
    "bead": "supabyoi-pks",
    "project": "supabyoi",
    "worktree": "/Users/you/worktrees/toast",
    "branch": "supabyoi-pks",
    "port_offset": 1,
    "created_at": "2026-01-19T08:30:00Z",
    "last_activity": "2026-01-19T10:45:00Z",
    "status": "working"
  },
  "shadow": {
    "bead": "supabyoi-g4a",
    "project": "supabyoi",
    "worktree": "/Users/you/worktrees/shadow",
    "branch": "supabyoi-g4a",
    "port_offset": 2,
    "created_at": "2026-01-19T09:15:00Z",
    "last_activity": "2026-01-19T09:20:00Z",
    "status": "idle"
  }
}
```

---

## Commands Reference

### Hub Commands

| Command | Purpose |
|---------|---------|
| `wt` / `wt list` | List all active worker sessions |
| `wt new <bead-id>` | Spawn a new worker session for a bead |
| `wt <name-or-bead>` | Switch to a worker session |
| `wt watch` | Live monitoring dashboard |
| `wt kill <name>` | Terminate session without closing bead |
| `wt close <name>` | Complete work and clean up session |
| `wt hub` | Start or attach to the hub session |
| `wt projects` | List registered projects |
| `wt project add <name> <path>` | Register a new project |
| `wt project config <name>` | Edit project configuration |

### Worker Commands

| Command | Purpose |
|---------|---------|
| `wt status` | Show current session info |
| `wt done` | Mark work complete, prepare for merge |
| `wt abandon` | Discard work and close session |
| `wt signal <status> [message]` | Update session status |

---

## Seance (Past Session Queries)

Seance lets you talk to predecessor sessions. Instead of parsing logs, you can ask directly:
- "Why did you make this decision?"
- "Where were you stuck?"
- "What did you try that didn't work?"

### How It Works

1. When sessions start, wt logs the Claude session ID to `~/.config/wt/events.jsonl`
2. `wt seance` lists recent sessions (completed or killed)
3. `wt seance <name>` forks the session using `claude --resume <id>`
4. You can ask questions without modifying the original session

### Event Log (`~/.config/wt/events.jsonl`)

```jsonl
{"type":"session_start","name":"toast","bead":"supabyoi-pks","session_id":"abc123","timestamp":"2026-01-19T08:30:00Z"}
{"type":"session_end","name":"toast","bead":"supabyoi-pks","session_id":"abc123","status":"completed","timestamp":"2026-01-19T13:00:00Z"}
```

---

## Merge Modes

Configured per-project in `merge_mode`:

### `direct`
Push directly to main. No PR, no review.

Best for: Solo projects, prototypes, experiments.

### `pr-auto`
Create PR, auto-merge if CI passes.

Best for: Solo projects with CI, trusted automation.

### `pr-review` (Default)
Create PR, wait for human review.

Best for: Team projects, code that needs review.

---

## Test Environment

### Port Isolation

Workers get sequential port offsets (1, 2, 3...). Configure your docker-compose.yml to use them:

```yaml
services:
  db:
    image: postgres:15
    ports:
      - "${PORT_OFFSET:-0}5432:5432"
    environment:
      POSTGRES_DB: myapp_${PORT_OFFSET:-dev}

  api:
    build: .
    ports:
      - "${PORT_OFFSET:-0}3000:3000"
    environment:
      DATABASE_URL: postgres://localhost:${PORT_OFFSET:-0}5432/myapp_${PORT_OFFSET:-dev}
```

With PORT_OFFSET=1: ports 15432, 13000
With PORT_OFFSET=2: ports 25432, 23000

### Lifecycle

```
wt new supabyoi-pks
  │
  ├─→ Create worktree
  ├─→ Set PORT_OFFSET=1
  ├─→ Run on_create hooks (npm install)
  ├─→ Run test_env.setup (docker compose up -d)
  ├─→ Run test_env.health_check (wait for ready)
  └─→ Launch Claude

wt close toast
  │
  ├─→ Complete work (commit, push, PR)
  ├─→ Run on_close hooks
  ├─→ Run test_env.teardown (docker compose down)
  ├─→ Remove worktree
  └─→ Free port offset
```

---

## Idle Detection

### How It Works

1. Capture tmux pane output
2. Look for activity patterns:
   - "Using tool:", "Thinking", "Reading" → Working
   - Prompt waiting ("> ", "$ ") for >5min → Idle
   - "error", "failed" → Error
3. Track last activity timestamp in sessions.json

### Status Indicators

| Icon | Status | Meaning |
|------|--------|---------|
| 🟢 | Working | Claude actively using tools |
| 🟡 | Idle | Waiting for input >5min |
| 🔴 | Error | Error detected in output |
| ⚫ | No session | Tmux session doesn't exist |

---

## Beads Integration

### BEADS_DIR

Each worker session has `BEADS_DIR` set to the main repo's `.beads/`:

```bash
# Session: toast
# Worktree: ~/worktrees/toast/ (for project supabyoi)
# BEADS_DIR: ~/supabyoi/.beads

# All bd commands in this session use the main repo's beads:
bd show supabyoi-pks    # Works
bd close supabyoi-pks   # Works
bd ready                # Shows project's ready beads
```

### Bead Lifecycle

```
bd create "New feature"     # In hub: creates bead (open)
       │
       ▼
wt new supabyoi-xyz         # Spawns session
       │                    # Bead → in_progress
       ▼
   [Claude works]
       │
       ▼
wt done                     # Bead → awaiting_review (if PR)
       │                    # Or → closed (if direct merge)
       ▼
[PR merged manually]        # (if pr-review mode)
       │
       ▼
wt close toast              # Cleanup session
                            # Bead already closed or closes now
```

---

## Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Hub session | Optional (`wt hub`) | Can orchestrate from any terminal, dedicated hub is convenience |
| Multiple workers same bead | Block | One bead = one session, enforce it |
| Auto-cleanup on merge | No | Manual `wt close`, you control lifecycle |
| Session restore after reboot | No | Start fresh, state file is just tracking |
| Namepool exhaustion | Error | 20 names is plenty, clean up old sessions |
| Signal-based communication | Pull model | Workers signal status, hub polls—no complex pub/sub |
| Context handoff | Explicit | `wt handoff` is manual, not automatic on context overflow |
| Auto mode | Opt-in | Autonomous processing requires explicit `wt auto`, not default |

These decisions favor explicitness and simplicity over automation and magic.
