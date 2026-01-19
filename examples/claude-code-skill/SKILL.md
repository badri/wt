---
name: wt-session-manager
description: Manage bead-driven worktree sessions from the hub. Spawn isolated worker sessions, monitor progress, and orchestrate multi-session development workflows. Use this skill when managing parallel Claude workers on different beads.
---

# wt - Worktree Session Manager

## Overview

wt orchestrates isolated development sessions where each bead gets its own:
- **Git worktree** - Isolated code environment
- **Tmux session** - Persistent terminal
- **Test environment** - Docker containers with port isolation
- **Claude agent** - Working on the bead

**Philosophy**: One bead = one session = one worktree. Sessions persist until explicitly closed.

## Architecture: Hub and Workers

```
HUB SESSION (You are here)
├── Groom beads: wt create, wt ready, wt beads
├── Spawn workers: wt new <bead-id>
├── Monitor: wt, wt watch
└── Switch: wt <session-name>
         │
         ▼
┌─────────────────────────────────────┐
│  WORKER: toast                       │
│  Bead: project-abc                   │
│  Worktree: ~/worktrees/toast/        │
│  Claude running, working...          │
└─────────────────────────────────────┘
┌─────────────────────────────────────┐
│  WORKER: shadow                      │
│  Bead: project-xyz                   │
│  Worktree: ~/worktrees/shadow/       │
│  Claude running, working...          │
└─────────────────────────────────────┘
```

**Hub session**: Where you groom beads and orchestrate workers (this session)
**Worker sessions**: Isolated Claude instances working on specific beads

## When to Use wt

### Use wt when:
- **Parallel work** - Multiple beads need simultaneous attention
- **Isolation needed** - Changes shouldn't interfere with each other
- **Test environments** - Each task needs its own Docker/ports
- **Long-running tasks** - Work that persists across hub compaction

### Don't use wt when:
- **Single simple task** - Just work directly in current session
- **Quick fix** - Doesn't need isolation or persistence
- **No worktree needed** - Task doesn't involve code changes

**Key insight**: wt adds overhead (worktree, tmux, test env). Use it when isolation and parallelism justify the cost.

## Session Start Protocol (Hub)

At session start, check for active workers and ready beads:

### Session Start Checklist

```
Hub Session Start:
- [ ] Run wt to see active worker sessions
- [ ] Check session status: idle, working, error
- [ ] Run bd ready to see available work
- [ ] Report to user: "X active workers, Y beads ready"
- [ ] If idle workers: suggest checking on them
```

**Pattern**: Always check both `wt` (active sessions) AND `bd ready` (available work). Idle workers may need attention.

**Report format**:
- "You have X active workers: [summary]. Y beads are ready for new workers."
- "Worker 'toast' has been idle for 15 minutes - want me to check on it?"

---

## Core Operations

### Listing Sessions

```bash
wt              # List all active sessions
wt list         # Same as above
```

Output shows: name, bead, status (working/idle/error), last activity, title

### Spawning Workers

```bash
wt new <bead-id>                    # Spawn worker, auto-switch
wt new <bead-id> --no-switch        # Spawn but stay in hub
wt new <bead-id> --name custom      # Use specific name
wt new <bead-id> --repo ~/project   # Explicit project path
```

**What happens on spawn:**
1. Allocates session name from pool (toast, shadow, obsidian...)
2. Creates git worktree at `~/worktrees/<name>/`
3. Creates branch named after bead
4. Sets up test environment (docker compose up)
5. Launches Claude in tmux session
6. Switches to the new session (unless --no-switch)

**Important**: One bead = one session. Cannot spawn multiple workers for same bead.

### Switching Sessions

```bash
wt <name>           # Switch by session name
wt <bead-id>        # Switch by bead (looks up session)
wt toast            # Example: switch to toast
```

Attaches to the tmux session. Use `Ctrl-b d` to detach back to hub.

### Monitoring

```bash
wt watch            # Live dashboard, refreshes every 30s
wt watch --notify   # Desktop notifications on idle
```

Watch shows:
- All active sessions with status
- Idle detection (!! marker when idle >5 min)
- PRs pending review

### Checking Session Status

```bash
wt status           # Current session info (from inside worker)
```

Shows: session name, bead, project, worktree path, branch, port offset, status

### Checking Ready Beads

```bash
wt ready                    # All registered projects (aggregated)
wt ready <project>          # Specific project only
```

Lists beads ready for work (no blockers, not in progress).

**Multi-project aggregation**: When called without a project filter, `wt ready` queries all registered projects and shows a combined view. This is the hub's unified view of available work.

---

## Multi-Project Bead Grooming

The hub can create and query beads across any registered project without changing directories.

### Creating Beads in Any Project

```bash
wt create <project> <title> [options]
wt create foo-frontend "Implement login API consumer"
wt create foo-backend "Add /users endpoint" -p 1 -t feature
wt create myapp "Fix auth bug" -d "Token refresh fails on timeout" -t bug
```

**Options:**
- `-d, --description` - Detailed description
- `-p, --priority` - Priority (0=critical, 1=high, 2=normal, 3=low)
- `-t, --type` - Issue type (task, bug, feature, epic, chore)

**What happens:**
1. Looks up project in registered projects
2. Creates bead in that project's `.beads/` directory
3. Bead lives in the project - wt just provides hub access

### Listing Beads for a Project

```bash
wt beads <project>                    # All beads
wt beads <project> --status open      # Filter by status
wt beads foo-frontend --status open
```

### Cross-Project Workflow Example

```bash
# Working on backend, realize frontend needs work too
wt beads foo-backend                  # See backend beads
wt create foo-frontend "Consume new /users endpoint" -d "Backend endpoint done in foo-backend-xyz"

# Now you can spawn workers for either
wt new foo-backend-abc --no-switch
wt new foo-frontend-def --no-switch
```

**Key insight**: The project's `.beads/` remains source of truth. `wt` is just a multi-project interface that knows where each project lives.

---

## Session Lifecycle

### Completing Work

**From worker session:**
```bash
wt done                     # Commit, push, create PR
wt done --merge-mode direct # Force direct merge
```

**From hub:**
```bash
wt close <name>             # Complete + cleanup session
```

**What `wt done` does:**
1. Checks for uncommitted changes (blocks if present)
2. Pushes branch to remote
3. Creates PR (based on merge mode)
4. Updates bead status

**Merge modes:**
- `direct` - Merge directly to main, no PR
- `pr-auto` - Create PR, auto-merge when CI passes
- `pr-review` - Create PR, wait for human review (default)

### Killing Sessions

```bash
wt kill <name>              # Stop session, keep bead open
wt kill <name> --keep-worktree  # Keep worktree too
```

Use when: need to restart session, or task is blocked

### Abandoning Work

```bash
wt abandon                  # From inside worker
```

Discards all changes, removes worktree, keeps bead open.

---

## Project Management

### Listing Projects

```bash
wt projects                 # List registered projects
```

Shows: name, repo path, merge mode, active session count

### Registering Projects

```bash
wt project add <name> <path>
wt project add myapp ~/myapp
```

### Configuring Projects

```bash
wt project config <name>    # Opens in $EDITOR
```

**Project config options:**
- `merge_mode` - direct, pr-auto, pr-review
- `default_branch` - usually main
- `beads_prefix` - for bead matching
- `test_env.setup` - docker compose up command
- `test_env.teardown` - docker compose down command
- `hooks.on_create` - run on session create
- `hooks.on_close` - run on session close

---

## Seance: Talking to Past Sessions

Query past Claude sessions to understand decisions and context.

### List Past Sessions

```bash
wt seance                   # List recent sessions
wt seance --project myapp   # Filter by project
wt seance --recent 10       # Last 10 sessions
```

### Talk to Past Session

```bash
wt seance <name>            # Interactive conversation
wt seance toast -p "What blocked you?"  # One-shot question
```

**Use cases:**
- "Why did you make this decision?"
- "Where were you stuck?"
- "What did you try that didn't work?"

---

## Common Patterns

### Pattern 1: Morning Workflow

```bash
# Check state
wt                          # See active workers
bd ready                    # See available work

# Spawn workers for ready beads
wt new project-abc --no-switch
wt new project-xyz --no-switch

# Monitor throughout the day
wt watch
```

### Pattern 2: Check on Idle Worker

```bash
# From wt list or watch, see "shadow" is idle
wt shadow                   # Switch to it
# Check what's happening, nudge Claude if needed
# Ctrl-b d to return to hub
```

### Pattern 3: Complete and Cleanup

```bash
# From hub, close completed session
wt close toast              # Completes work + cleanup

# Or from inside worker
wt done                     # Submit work
wt close                    # Then cleanup
```

### Pattern 4: Parallel Development Sprint

```bash
# Spawn multiple workers
wt new app-feature-1 --no-switch
wt new app-feature-2 --no-switch
wt new app-bugfix-3 --no-switch

# Monitor all
wt watch --notify

# Switch to check progress
wt app-feature-1
# Review, give guidance
# Ctrl-b d

wt app-feature-2
# Review, give guidance
# Ctrl-b d
```

### Pattern 5: Resume After Session Start

If hub session restarts (compaction, new session), workers persist:

```bash
# Workers are still running in tmux
wt                          # See them all
wt toast                    # Resume interaction
```

**Key benefit**: Worker sessions survive hub compaction.

---

## Integration with bd

### bd for Strategic Work, wt for Execution

```bash
# In hub: groom beads
bd ready                    # What's available?
bd show project-abc         # Review details

# Spawn worker
wt new project-abc          # Execute in isolation

# Worker updates bead automatically
# - Sets status to in_progress on spawn
# - Creates PR on wt done
# - Closes bead on wt close
```

### BEADS_DIR Inheritance

Workers inherit `BEADS_DIR` from the project, so bd commands inside workers operate on the correct project's beads.

---

## Troubleshooting

**Session won't spawn:**
- Check if bead exists: `bd show <bead-id>`
- Check if already has a session: `wt` (one bead = one session)
- Check project registered: `wt projects`

**Can't find session by bead:**
- Use `wt` to list all sessions
- Session-bead mapping in `~/.config/wt/sessions.json`

**Worker seems stuck:**
- `wt watch` to check status
- `wt <name>` to switch and investigate
- `wt kill <name>` to restart if needed

**Port conflicts:**
- Each session gets unique PORT_OFFSET (1, 2, 3...)
- Check `wt status` for assigned offset
- Ensure docker-compose.yml uses PORT_OFFSET

**Session cleanup issues:**
- `wt kill <name>` to force-stop
- Check for orphaned worktrees: `git worktree list`
- Check for orphaned tmux: `tmux list-sessions`

---

## Quick Reference

| Command | Description |
|---------|-------------|
| `wt` | List active sessions |
| `wt new <bead>` | Spawn worker for bead |
| `wt <name>` | Switch to session |
| `wt watch` | Live monitoring |
| `wt status` | Current session info (in worker) |
| `wt done` | Submit work (in worker) |
| `wt close <name>` | Complete + cleanup |
| `wt kill <name>` | Terminate session |
| `wt abandon` | Discard work (in worker) |
| `wt projects` | List projects |
| `wt project add` | Register project |
| `wt ready` | Show ready beads (all projects) |
| `wt ready <project>` | Show ready beads (one project) |
| `wt create <project> <title>` | Create bead in project |
| `wt beads <project>` | List beads for project |
| `wt seance` | List past sessions |
| `wt seance <name>` | Talk to past session |

---

## Reference Files

| Reference | Read When |
|-----------|-----------|
| [references/CLI_REFERENCE.md](references/CLI_REFERENCE.md) | Need complete command reference with all flags |
| [references/WORKFLOWS.md](references/WORKFLOWS.md) | Need step-by-step workflows with checklists |
| [references/HUB_PATTERNS.md](references/HUB_PATTERNS.md) | Need detailed hub orchestration patterns |
