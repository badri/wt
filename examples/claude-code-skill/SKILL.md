---
name: wt-session-manager
description: Manage bead-driven worktree sessions from the hub. Spawn isolated worker sessions, monitor progress, and orchestrate multi-session development workflows. Aggregate and list work across multiple registered projects (`wt ready`, `wt beads`, `wt projects`). Use this skill for "ready work?", "what's ready?", "available tasks?", or any query about ready/available work when in a hub session or when aggregating across projects. Preferred over beads:ready in hub context.
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
wt list --json  # JSON output for scripting/LLM consumption
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
wt watch            # Interactive TUI dashboard
```

**Watch TUI controls:**
- `↑/↓` or `j/k` - Navigate between sessions
- `Enter` - Switch to selected session
- `r` - Refresh manually
- `q` - Quit watch

**Watch shows:**
- All active sessions with color-coded status
- Status: working (green), idle (yellow), ready (bright green), blocked/error (red)
- Status message or idle time
- Auto-refreshes every 5 seconds without flashing

**Tmux pane navigation** (when watch is in side pane):
- `Ctrl+b ←/→` - Switch between Claude and watch panes
- `Ctrl+b o` - Cycle through panes

### Checking Session Status

```bash
wt status           # Current session info (from inside worker)
wt status --json    # JSON output for scripting/LLM consumption
```

Shows: session name, bead, project, worktree path, branch, port offset, status

### Checking Ready Beads

```bash
wt ready                    # All registered projects (aggregated)
wt ready <project>          # Specific project only
wt ready --json             # JSON output for scripting/LLM consumption
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
wt beads <project> --json             # JSON output for scripting/LLM consumption
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

## Task Sessions (Lightweight, Non-Bead)

Task sessions are lightweight alternatives to bead sessions for transient work that doesn't need issue tracking.

### When to Use Tasks vs Beads

| Use Tasks | Use Beads |
|-----------|-----------|
| Quick investigations | Strategic work |
| MCP queries | Multi-session work |
| PR conflict resolution | Work with dependencies |
| Temporary experiments | Needs tracking/history |

### Creating Task Sessions

```bash
wt task <description>                           # Simple task, no conditions
wt task "Investigate slow query" --condition none
wt task "Fix PR conflicts" --condition user-confirm
wt task "Run integration tests" --condition tests-pass --project myapp
wt task "Push hotfix" --condition pushed
wt task "Submit PR" --condition pr-merged
```

**Completion conditions:**
- `none` - No checks, completes immediately with `wt done`
- `pushed` - Verifies changes are pushed to remote
- `pr-merged` - Waits for PR to be merged
- `tests-pass` - Runs test suite before completing
- `user-confirm` - Requires user confirmation

**Task workflow:**
1. Hub spawns task: `wt task "description" --condition X`
2. Task runs in isolated worktree with branch `task/<description>`
3. Worker completes task and signals: `wt signal ready "message"`
4. Hub verifies condition and runs: `wt done`

### Listing Tasks

Tasks appear in `wt list` with type "task":

```
Active Sessions

     Name               Type   Status      Title                           Project
────────────────────────────────────────────────────────────────────────────────────
     wt-toast           bead   working     Add login feature               myapp
     myapp-task-shadow  task   ready       Fix PR conflicts                myapp
```

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
wt projects --json          # JSON output for scripting/LLM consumption
```

Shows: name, repo path, merge mode, active session count

### Registering Projects

When a user asks to register a project, **always ask about the merge mode** before completing registration:

1. Ask the user which merge mode they prefer:
   - `direct` - Merge directly to main, no PR (fast, for solo work)
   - `pr-auto` - Create PR, auto-merge when CI passes (balanced)
   - `pr-review` - Create PR, wait for human review (default, safest)

2. Register the project:
   ```bash
   wt project add <name> <path>
   ```

3. Configure the merge mode:
   ```bash
   wt project config <name>    # Opens in $EDITOR, set merge_mode
   ```

**Example conversation:**
```
User: "Register ~/code/myapp"
You: "I'll register that. How should completed work be merged?
      - direct: Merge directly to main (no PR)
      - pr-auto: Create PR, auto-merge when CI passes
      - pr-review: Create PR, wait for human review"
User: "pr-auto"
You: [register project and set merge_mode to pr-auto]
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
wt seance                   # List recent sessions (workers + hub)
```

Output shows Session, Title, Project, and Time. Sessions are logged when they end via `wt done`, `wt close`, or `wt kill`. Hub sessions are logged on `wt handoff`.

### Talk to Past Session

```bash
wt seance <name>            # Resume in new tmux pane (safe from hub)
wt seance <name> --spawn    # Spawn new tmux session
wt seance toast -p "What blocked you?"  # One-shot question
```

### Resume Hub Sessions

```bash
wt seance hub --spawn       # Resume most recent hub in new tmux session
```

**Use cases:**
- "Why did you make this decision?"
- "Where were you stuck?"
- "What did you try that didn't work?"

---

## Autonomous Batch Processing

Run beads overnight or unattended with `wt auto`.

### Basic Usage

```bash
wt auto                     # Process all ready beads across all projects
wt auto --project myapp     # Only beads from specific project
wt auto --dry-run           # Preview what would run
wt auto --check             # Validate beads are well-groomed
```

### How It Works

1. Acquires lock (`~/.config/wt/auto.lock`) - only one auto run at a time
2. Gets ready beads from all projects (or filtered by `--project`)
3. For each bead:
   - Creates session: `wt new <bead> --no-switch`
   - Runs Claude with prompt template
   - Waits for completion or timeout
   - Logs result to `~/.config/wt/logs/auto-<timestamp>.log`
4. Checks for stop signal between beads
5. Continues until all beads processed

### Flags

| Flag | Description |
|------|-------------|
| `--project <name>` | Filter to specific project |
| `--dry-run` | Show what would run without executing |
| `--check` | Validate beads have title/description |
| `--stop` | Signal running auto to stop after current bead |
| `--timeout <minutes>` | Override default 30min timeout |
| `--merge-mode <mode>` | Override project merge mode |
| `--force` | Override lock (risky) |

### Stopping a Running Auto

```bash
wt auto --stop              # Graceful stop after current bead
```

Or create stop file: `touch ~/.config/wt/stop-auto`

The current bead continues to completion, then auto exits.

### Logs

Auto runs are logged to `~/.config/wt/logs/auto-<timestamp>.log`:
- Start/end times
- Per-bead: ID, outcome (success/fail/timeout), duration

### When to Use

- **Overnight batch**: Groom beads during day, run auto overnight
- **CI integration**: Trigger auto from CI pipeline
- **Backlog clearing**: Process accumulated ready beads

### Example: Overnight Workflow

```bash
# During day: groom beads
bd ready                    # Review what's ready
bd show project-xyz         # Check each has good description

# Before leaving
wt auto --check             # Verify all groomed
wt auto                     # Start batch run

# Next morning
cat ~/.config/wt/logs/auto-*.log | tail -50  # Check results
wt                          # See any sessions still running
```

---

## Event Monitoring

Track what's happening across worker sessions with `wt events`.

### Basic Usage

```bash
wt events                   # Show recent 20 events
wt events --tail            # Follow mode (like tail -f)
wt events --since 5m        # Events from last 5 minutes
wt events --since 1h        # Events from last hour
```

### Events Logged

- `session_start` - Worker session created
- `session_end` - Worker completed work (includes PR URL if created)
- `session_kill` - Worker was killed
- `pr_created` - Pull request created
- `pr_merged` - Pull request merged

### Hook Integration

Use `--new --clear` for Claude Code hook integration:

```bash
wt events --new --clear     # Show new events, mark as read
```

**Claude Code settings.json:**
```json
{
  "hooks": {
    "prompt-submit": ["wt events --new --clear"]
  }
}
```

This automatically reports events at the start of each conversation turn:
- "wt session 'toast' completed bead wt-xyz (PR: https://...)"
- "wt session 'shadow' was killed (bead: wt-abc)"

### When to Use

- **Monitor workers**: See what happened while you were away
- **Hub notifications**: Auto-report via hooks without manually checking
- **Debugging**: Track session lifecycle events

---

## Diagnostics

Check your wt setup with `wt doctor`.

### Basic Usage

```bash
wt doctor                   # Run all diagnostic checks
```

### Checks Performed

| Check | Description |
|-------|-------------|
| **tmux** | Installed, version, server running |
| **git** | Installed, version |
| **beads (bd)** | bd command installed, version |
| **worktree root** | Directory exists and is writable |
| **config** | Config file valid, no empty required values |
| **orphaned sessions** | Sessions in state but no tmux session |
| **orphaned worktrees** | Worktree directories without active sessions |
| **missing worktrees** | Sessions referencing non-existent worktrees |

### Example Output

```
┌─ wt doctor ───────────────────────────────────────────────────────────┐
│                                                                       │
│  [✓] tmux: tmux 3.5a, server running (3 sessions)                     │
│  [✓] git: version 2.50.1                                              │
│  [✓] beads (bd): bd version 0.47.1                                    │
│  [✓] worktree root: /Users/me/worktrees exists and is writable        │
│  [✓] config: using defaults (no config.json)                          │
│  [✓] sessions: 2 active session(s), no orphans                        │
│                                                                       │
└───────────────────────────────────────────────────────────────────────┘

All checks passed!
```

### Status Indicators

- `[✓]` - Check passed
- `[!]` - Warning (non-critical issue)
- `[✗]` - Error (must be fixed)

### When to Use

- **Initial setup**: Verify wt is configured correctly
- **After issues**: Diagnose problems with sessions or worktrees
- **Cleanup**: Find orphaned sessions/worktrees to clean up

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

**First step for any issue:**
- Run `wt doctor` to check for common problems

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
- Run `wt doctor` to find orphaned sessions/worktrees
- `wt kill <name>` to force-stop sessions
- Check for orphaned worktrees: `git worktree list`
- Check for orphaned tmux: `tmux list-sessions`

---

## Machine-Readable Output (--json)

Most listing commands support `--json` for machine-readable output, useful for:
- **LLM/AI agent consumption** - Skills, automation, intelligent decision making
- **Scripting and pipelines** - Parse output with jq, integrate into scripts
- **Integration with other tools** - Build custom dashboards, notifications

### Commands with JSON support

```bash
wt list --json              # Sessions array
wt projects --json          # Projects array with session counts
wt ready --json             # Ready beads array
wt beads <project> --json   # Project beads array
wt status --json            # Current session object
```

### Example: Parse with jq

```bash
# Get names of all active sessions
wt list --json | jq -r '.[].name'

# Get ready beads with priority 0 (critical)
wt ready --json | jq '.[] | select(.priority == 0)'

# Count sessions per project
wt list --json | jq 'group_by(.project) | map({project: .[0].project, count: length})'
```

---

## Quick Reference

| Command | Description |
|---------|-------------|
| `wt` | List active sessions |
| `wt --json` | List sessions as JSON (for scripting/LLM) |
| `wt new <bead>` | Spawn worker for bead (stays in hub) |
| `wt new <bead> --switch` | Spawn and switch to worker |
| `wt new <bead> --no-test-env` | Spawn without test env setup |
| `wt task <desc>` | Spawn lightweight task session |
| `wt task <desc> --condition X` | Task with completion condition |
| `wt <name>` | Switch to session |
| `wt watch` | Interactive TUI (↑↓ navigate, Enter switch, q quit) |
| `wt status` | Current session info (in worker) |
| `wt status --json` | Session status as JSON |
| `wt signal ready "msg"` | Signal work complete (in worker) |
| `wt signal blocked "msg"` | Signal blocked (in worker) |
| `wt signal error "msg"` | Signal error (in worker) |
| `wt done` | Submit work (in worker) |
| `wt close <name>` | Complete + cleanup |
| `wt kill <name>` | Terminate session |
| `wt abandon` | Discard work (in worker) |
| `wt projects` | List projects |
| `wt projects --json` | List projects as JSON |
| `wt project add` | Register project |
| `wt ready` | Show ready beads (all projects) |
| `wt ready --json` | Ready beads as JSON |
| `wt ready <project>` | Show ready beads (one project) |
| `wt create <project> <title>` | Create bead in project |
| `wt beads <project>` | List beads for project |
| `wt beads <project> --json` | Project beads as JSON |
| `wt seance` | List past sessions (workers + hub) |
| `wt seance <name>` | Resume in new tmux pane |
| `wt seance <name> --spawn` | Resume in new tmux session |
| `wt seance hub --spawn` | Resume last hub session |
| `wt auto` | Process ready beads autonomously |
| `wt auto --dry-run` | Preview auto run |
| `wt auto --stop` | Stop running auto gracefully |
| `wt events` | Show recent events |
| `wt events --tail` | Follow events in real-time |
| `wt events --new --clear` | Get new events (for hooks) |
| `wt doctor` | Diagnose setup issues |
| `wt hub` | Create or attach to hub session (with watch pane) |
| `wt hub --no-watch` | Create hub without watch pane |
| `wt hub --status` | Show hub status without attaching |
| `wt hub --detach` | Detach from hub (return to previous) |
| `wt hub --kill` | Kill hub session (with confirmation) |
| `wt handoff` | Handoff hub to fresh Claude instance |
| `wt config` | Show/manage wt configuration |
| `wt prime` | Inject startup context (for hooks) |

---

## Hub Session: Dedicated Orchestration

The hub is a dedicated tmux session for orchestrating worker sessions. Unlike worker sessions, the hub has no worktree and is not tied to any specific bead.

### Creating/Attaching to Hub

```bash
wt hub                      # Create hub or attach if exists
wt hub --status             # Show hub status without attaching
wt hub --detach             # Detach from hub, return to previous session
wt hub --kill               # Kill hub session (prompts for confirmation)
wt hub --kill --force       # Kill hub without confirmation
wt hub --no-watch           # Create hub without watch pane
```

### Hub Characteristics

- **Session name**: Always "hub"
- **Working directory**: Home directory (~)
- **Watch pane**: Right pane with `wt watch` (25% width, skip with `--no-watch`)
- **No worktree**: Hub doesn't have code isolation
- **No BEADS_DIR**: Uses bd's project detection
- **Persistent**: Survives across Claude instances via handoff

### When to Use Hub

- **Central orchestration**: Manage multiple worker sessions from one place
- **Cross-project work**: Work on beads from different projects
- **Monitoring**: Watch workers, check events, review progress

### Hub Workflow

```bash
# Start or attach to hub
wt hub

# From hub, spawn workers (automatically stays in hub, workers start working)
wt new wt-abc           # Creates wt-woody, sends initial prompt, stays in hub
wt new supabyoi-xyz     # Creates supabyoi-buzz, starts working

# Monitor workers
wt watch

# Switch to a worker to check on it
wt wt-woody             # Ctrl-b d to detach back to hub

# When done, detach from hub
wt hub --detach         # Returns to previous session
```

**Key behaviors from hub:**
- `wt new` stays in hub by default (use `--switch` to attach)
- Workers receive detailed workflow instructions and start working
- Session names are project-prefixed: `wt-woody`, `supabyoi-buzz`
- Workers do NOT run `wt done` - hub handles cleanup after review

**Worker instructions include:**
1. Implement the task
2. Commit changes with descriptive message
3. Run tests (if configured)
4. Create PR (or push for direct mode)
5. Signal completion: `wt signal ready "PR: <url>"`

**Signaling status:**
```bash
wt signal ready "PR: https://github.com/org/repo/pull/123"
wt signal blocked "Waiting for API credentials"
wt signal error "Tests failing on CI"
```

Hub sees status changes in `wt watch` and receives notifications.

### Hub vs Worker Sessions

| Aspect | Hub | Worker |
|--------|-----|--------|
| Session name | "hub" | Project-prefixed (wt-woody, etc.) |
| Working dir | ~ | Worktree path |
| Worktree | None | Yes, isolated |
| BEADS_DIR | Not set | Set to project's .beads |
| Purpose | Orchestration | Actual coding work |

### Hub Sessions in Seance

Hub sessions appear in `wt seance` after a handoff, allowing you to resume previous hub conversations:

```bash
wt seance                   # Lists both worker (⚙️) and hub (🏠) sessions
wt seance hub --spawn       # Resume the last hub session
```

**Example output:**
```
Past Sessions (seance)

     Session             Title                                 Project         Time
────────────────────────────────────────────────────────────────────────────────────
 ⚙️  myproject-toast     Add OAuth authentication flow         myproject       2026-01-20 14:30
 🏠  hub                                                                       2026-01-20 12:00

⚙️ = Worker session   🏠 = Hub session
```

---

## Hub Persistence: Handoff and Prime

Hub sessions can survive compaction and restarts through the handoff system.

### The Problem

When the hub session compacts or restarts:
- Worker sessions survive (they're in tmux)
- But hub context is lost

### The Solution: wt handoff

**From hub session:**
```bash
wt handoff                  # Cycle to fresh Claude, preserve context
wt handoff -m "notes"       # Include custom message
wt handoff -c               # Auto-collect state (sessions, ready beads)
wt handoff --dry-run        # Preview what would be collected
```

**What happens:**
1. Collects context (active sessions, ready beads, in-progress work)
2. Stores in "Hub Handoff" bead (persists in beads)
3. Logs hub session for seance (can resume via `wt seance hub`)
4. Writes handoff marker file
5. Clears tmux history
6. Respawns fresh Claude via `tmux respawn-pane`

### Startup: wt prime

**New Claude session runs:**
```bash
wt prime                    # Inject context from previous session
wt prime --quiet            # Suppress non-essential output
wt prime --no-bd-prime      # Skip running bd prime
```

**What happens:**
1. Checks for handoff marker (detects post-handoff state)
2. Shows warning: "DO NOT run /handoff - that was your predecessor"
3. Injects handoff content from bead
4. Runs `bd prime` for beads context

### Claude Code Hook Integration

Add to `.claude/settings.json`:
```json
{
  "hooks": {
    "SessionStart": ["wt prime"]
  }
}
```

This auto-primes new sessions with handoff context.

### When to Use Handoff

- **Context bloat**: Session growing slow, need fresh start
- **Before leaving**: Save state before stepping away
- **After major decision**: Checkpoint before moving on

### Key Points

- Worker sessions survive handoff (they're in tmux)
- Handoff bead persists context in beads database
- Marker file prevents "handoff loop" bug
- `wt prime` clears marker after reading

---

## Reference Files

| Reference | Read When |
|-----------|-----------|
| [references/CLI_REFERENCE.md](references/CLI_REFERENCE.md) | Need complete command reference with all flags |
| [references/WORKFLOWS.md](references/WORKFLOWS.md) | Need step-by-step workflows with checklists |
| [references/HUB_PATTERNS.md](references/HUB_PATTERNS.md) | Need detailed hub orchestration patterns |
