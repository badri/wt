# wt - Worktree Session Manager Specification

## Overview

`wt` is a minimal agentic coding orchestrator built on:
- **Beads** for task tracking
- **Git worktrees** for isolation
- **Tmux** for session persistence
- **Claude** (or other agents) for execution

**Philosophy**: One bead = one session = one worktree. Sessions persist until you explicitly close them. No auto-compaction, no handoff complexity.

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

## Commands

### Hub Commands (run from grooming session)

#### `wt` / `wt list`
List all active worker sessions.

```
$ wt
┌─ Active Sessions ───────────────────────────────────────────────────────┐
│                                                                         │
│  Name       Bead              Status    Last Activity   Title           │
│  ────       ────              ──────    ─────────────   ─────           │
│  🟢 toast    supabyoi-pks     Working   2m ago          Auto-harden VM  │
│  🟡 shadow   supabyoi-g4a     Idle      15m ago  !!     Encrypt secrets │
│  🟢 obsidian reddit-saas-8lr  Working   1m ago          Supabase setup  │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘

Commands: wt <name> (switch) | wt new <bead> | wt close <name>
```

#### `wt new <bead-id> [--name <name>]`
Spawn a new worker session for a bead.

```bash
$ wt new supabyoi-pks
Spawning worker session...
  Name:      toast (from pool)
  Bead:      supabyoi-pks
  Project:   supabyoi
  Worktree:  ~/worktrees/toast/
  Branch:    supabyoi-pks

Creating git worktree...
Setting up test environment...
  Running: docker compose up -d
  Port offset: 1 (ports: 15432, 13000)
Launching Claude...

Session 'toast' ready. Switching...
```

Options:
- `--name <name>`: Use specific name instead of pool
- `--no-env`: Skip test environment setup
- `--no-switch`: Don't switch to session after creating

#### `wt <name-or-bead>`
Switch to a worker session (into Claude).

```bash
$ wt toast           # By session name
$ wt supabyoi-pks    # By bead (looks up session)
```

Attaches to the tmux session. You land in Claude's conversation.

#### `wt watch`
Live monitoring dashboard with idle detection.

```bash
$ wt watch
┌─ Sessions (refreshing every 30s) ───────────────────────────────────────┐
│                                                                         │
│  🟢 toast      supabyoi-pks     Working   2m ago          Auto-harden   │
│  🟡 shadow     supabyoi-g4a     IDLE      15m ago  !!     Encrypt       │
│  🟢 obsidian   reddit-saas-8lr  Working   1m ago          Supabase      │
│                                                                         │
│  PRs Pending Review:                                                    │
│    supabyoi#42  supabyoi-e5s   Deployment progress UI                   │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
[q]uit  [r]efresh  [Enter]switch to selected
```

Options:
- `--notify`: Desktop notification when session goes idle
- `--interval <secs>`: Refresh interval (default: 30)

#### `wt projects`
List registered projects.

```bash
$ wt projects
┌─ Projects ──────────────────────────────────────────────────────────────┐
│                                                                         │
│  Name          Repo                  Merge Mode    Active Sessions      │
│  ────          ────                  ──────────    ───────────────      │
│  supabyoi      ~/supabyoi            pr-review     2 (toast, shadow)    │
│  reddit-saas   ~/reddit-saas         direct        1 (obsidian)         │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

#### `wt project add <name> <path>`
Register a new project.

```bash
$ wt project add supabyoi ~/supabyoi
Project 'supabyoi' registered.
  Repo: ~/supabyoi
  Beads prefix: supabyoi
  Merge mode: pr-review (default)

Configure with: wt project config supabyoi
```

#### `wt project config <name>`
Edit project configuration (opens in $EDITOR).

#### `wt kill <name> [--keep-worktree]`
Terminate session without closing bead.

```bash
$ wt kill shadow
Killing session 'shadow'...
  Stopping test environment: docker compose down
  Removing worktree: ~/worktrees/shadow/
  Freeing name 'shadow' back to pool
Done. Bead supabyoi-g4a still open.
```

#### `wt close <name>`
Complete work: commit, push, create PR, close bead, cleanup.

```bash
$ wt close toast
Closing session 'toast'...
  Bead: supabyoi-pks

  Committing changes...
  Pushing branch supabyoi-pks...
  Creating PR (merge_mode: pr-review)...
    PR #45 created: https://github.com/you/supabyoi/pull/45

  Closing bead supabyoi-pks...
  Stopping test environment...
  Removing worktree...
  Freeing name 'toast' back to pool

Done.
```

### Worker Commands (run from inside a worker session)

#### `wt status`
Show current session info.

```bash
$ wt status
Session:   toast
Bead:      supabyoi-pks
Title:     Auto-harden VM security on add
Project:   supabyoi
Worktree:  ~/worktrees/toast/
Branch:    supabyoi-pks
Port:      1 (15432, 13000)
Status:    Working
```

#### `wt done`
Mark work complete, prepare for merge (but don't close session).

```bash
$ wt done
Completing work on supabyoi-pks...

  Committing changes...
  Pushing branch...
  Creating PR...
    PR #45: https://github.com/you/supabyoi/pull/45

  Marking bead as awaiting_review...

Work submitted. Session still active.
To close session: wt close
```

#### `wt abandon`
Discard work and close session.

```bash
$ wt abandon
WARNING: This will discard all uncommitted changes.
Continue? [y/N] y

Abandoning session 'toast'...
  Discarding changes...
  Removing worktree...
  Bead supabyoi-pks remains open.
Done.
```

### Navigation (Tmux shortcuts)

| Shortcut | Action |
|----------|--------|
| `C-b n` | Next worker session |
| `C-b p` | Previous worker session |
| `C-b h` | Return to hub (if configured) |
| `C-b w` | Session picker |

---

## Seance (Talk to Past Sessions)

Seance lets you talk to predecessor sessions. Instead of parsing logs, you can ask directly:
- "Why did you make this decision?"
- "Where were you stuck?"
- "What did you try that didn't work?"

### How It Works

1. When sessions start, wt logs the Claude session ID to `~/.config/wt/events.jsonl`
2. `wt seance` lists recent sessions (completed or killed)
3. `wt seance <name>` forks the session using `claude --resume <id>`
4. You can ask questions without modifying the original session

### Commands

#### `wt seance`
List recent sessions.

```bash
$ wt seance
┌─ Recent Sessions ───────────────────────────────────────────────────────┐
│                                                                         │
│  Name       Bead              Ended         Duration   Status           │
│  ────       ────              ─────         ────────   ──────           │
│  toast      supabyoi-pks      2h ago        4h 30m     Completed        │
│  shadow     supabyoi-g4a      1d ago        2h 15m     Killed           │
│  obsidian   reddit-saas-8lr   3d ago        6h 00m     Completed        │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘

Talk to a session: wt seance <name>
```

Options:
- `--project <name>`: Filter by project
- `--recent <n>`: Show last N sessions (default: 20)

#### `wt seance <name> [--prompt <question>]`
Talk to a past session.

```bash
# Interactive conversation (forks session)
$ wt seance toast
Resuming session 'toast' (bead: supabyoi-pks)...
Session ID: abc123-def456

[Claude session opens with full context from toast]
> Where did you put the nginx config?

# One-shot question
$ wt seance toast -p "What was blocking you?"
Resuming session 'toast'...

The main blocker was the DNS propagation delay. I implemented a
retry mechanism with exponential backoff in deploy.py:142...
```

### Event Log (`~/.config/wt/events.jsonl`)

```jsonl
{"type":"session_start","name":"toast","bead":"supabyoi-pks","session_id":"abc123","timestamp":"2026-01-19T08:30:00Z"}
{"type":"session_end","name":"toast","bead":"supabyoi-pks","session_id":"abc123","status":"completed","timestamp":"2026-01-19T13:00:00Z"}
{"type":"session_start","name":"shadow","bead":"supabyoi-g4a","session_id":"def456","timestamp":"2026-01-19T09:15:00Z"}
{"type":"session_end","name":"shadow","bead":"supabyoi-g4a","session_id":"def456","status":"killed","timestamp":"2026-01-19T11:30:00Z"}
```

### Implementation Notes

- Uses Claude's `--resume <session-id>` flag to fork a session
- Fork is read-only (doesn't modify original session's history)
- Session IDs captured from Claude's output on startup
- Events persisted even after session cleanup

---

## Merge Modes

Configured per-project in `merge_mode`:

### `direct`
Push directly to main. No PR, no review.

```bash
wt done
# → Commits to branch
# → Merges branch to main locally
# → Pushes main
# → Deletes branch
```

Best for: Solo projects, prototypes, experiments.

### `pr-auto`
Create PR, auto-merge if CI passes.

```bash
wt done
# → Pushes branch
# → Creates PR with auto-merge enabled
# → If CI green, PR merges automatically
# → Notifies when merged
```

Best for: Solo projects with CI, trusted automation.

### `pr-review`
Create PR, wait for human review.

```bash
wt done
# → Pushes branch
# → Creates PR
# → Notifies you
# → Waits for manual merge
```

Best for: Team projects, code that needs review.

---

## Test Environment

Each worktree can have its own isolated test environment.

### Port Isolation

Workers get sequential port offsets (1, 2, 3...). Configure your docker-compose.yml to use them:

```yaml
# docker-compose.yml
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

### Notification

When `wt watch --notify` is running:

```bash
# On macOS
osascript -e 'display notification "Session shadow is idle" with title "wt"'

# On Linux
notify-send "wt" "Session shadow is idle"
```

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

### Session-Bead Lookup

```bash
# Find session by bead
wt supabyoi-pks    # Looks up sessions.json, finds "toast", switches

# Find bead by session
wt status          # In session: shows bead info
```

---

## Skills (Claude Integration)

### `/wt` Skill

Install to `~/.claude/skills/wt.md`:

```markdown
# /wt - Worktree Session Manager

Manage bead-driven worktree sessions.

## From Hub (grooming session)

- `wt` - List all worker sessions
- `wt new <bead-id>` - Spawn worker for bead
- `wt <name>` - Switch to worker session
- `wt watch` - Live monitoring
- `wt close <name>` - Complete and cleanup

## From Worker (inside a session)

- `wt status` - Current session info
- `wt done` - Submit work (commit, push, PR)
- `wt close` - Done + cleanup session

## Workflow

1. In hub: `bd ready` to see available work
2. `wt new supabyoi-pks` to spawn worker
3. Work in Claude session
4. `wt done` when code complete
5. `wt close` to cleanup
```

---

## Tmux Configuration

Add to `~/.tmux.conf`:

```bash
# wt session navigation
bind-key n run-shell "wt next"
bind-key p run-shell "wt prev"
bind-key h run-shell "wt hub"  # Jump to hub (optional)

# Status line shows session name (= bead context)
set -g status-left "#[fg=cyan][#S] "
set -g status-right "#[fg=yellow] wt "

# Pane border shows session
set -g pane-border-format " #S "
set -g pane-border-status top
```

---

## Example Workflows

### Solo Developer, Single Project

```bash
# Morning: Start grooming session
$ claude
> bd ready
> wt new supabyoi-pks      # Spawns toast
> wt new supabyoi-g4a      # Spawns shadow

# Work on toast
> wt toast
[In Claude session, working...]

# Check on shadow
> C-b n                     # Switch to shadow
[Check progress, maybe nudge]

# Toast is done
> wt toast
> wt done                   # Creates PR
> wt close                  # Cleanup

# Back to hub
> C-b h
> wt                        # See remaining sessions
```

### Multiple Projects

```bash
# Register projects
$ wt project add supabyoi ~/supabyoi
$ wt project add reddit-saas ~/reddit-saas

# Spawn workers across projects
$ wt new supabyoi-pks       # toast
$ wt new reddit-saas-8lr    # shadow

# List shows all
$ wt
  toast    supabyoi-pks     Working   supabyoi
  shadow   reddit-saas-8lr  Working   reddit-saas

# Switch freely
$ wt toast
$ wt shadow
```

### Team Project with Review

```bash
# Project config: merge_mode = pr-review
$ wt new supabyoi-pks

# Work...
$ wt done
# → PR #45 created, waiting for review

# Session stays open for fixes
# Teammate reviews, requests changes

# Make fixes in same session
$ wt toast
[Make fixes]
$ git push                  # Updates PR

# PR approved and merged
$ wt close toast            # Cleanup
```

---

## Implementation Checklist

### Phase 1: Core
- [ ] `wt` (list sessions)
- [ ] `wt new <bead>` (spawn with namepool)
- [ ] `wt <name>` (switch to session)
- [ ] `wt kill <name>` (terminate)
- [ ] `wt close <name>` (complete + cleanup)
- [ ] Session state tracking (sessions.json)
- [ ] Namepool management
- [ ] BEADS_DIR per session

### Phase 2: Projects
- [ ] `wt projects` (list)
- [ ] `wt project add` (register)
- [ ] `wt project config` (edit)
- [ ] Per-project merge_mode
- [ ] Per-project test_env config

### Phase 3: Test Environment
- [ ] Port offset allocation
- [ ] test_env.setup on create
- [ ] test_env.teardown on close
- [ ] Health check waiting

### Phase 4: Merge Workflow
- [ ] `wt done` command
- [ ] direct mode (merge to main)
- [ ] pr-auto mode (create PR, auto-merge)
- [ ] pr-review mode (create PR, wait)

### Phase 5: Monitoring
- [ ] `wt watch` (live dashboard)
- [ ] Idle detection
- [ ] Desktop notifications
- [ ] PR status in dashboard

### Phase 6: Seance
- [ ] Event logging (session IDs)
- [ ] `wt seance` (list past sessions)
- [ ] `wt seance <name>` (talk to past session)
- [ ] Session ID to name mapping

### Phase 7: Polish
- [ ] Tmux keybindings
- [ ] Claude skill
- [ ] Shell completions
- [ ] Error handling
- [ ] Documentation

---

## Design Decisions (Locked)

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Hub session | Any terminal | No extra session to manage |
| Multiple workers same bead | Block | One bead = one session, enforce it |
| Auto-cleanup on merge | No | Manual `wt close`, you control lifecycle |
| Session restore after reboot | No | Start fresh, state file is just tracking |
| Namepool exhaustion | Error | 20 names is plenty, clean up old sessions |

**Philosophy**: Keep it simple. Extend later if needed.
