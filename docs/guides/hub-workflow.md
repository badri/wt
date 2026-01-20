# Hub Workflow

The **hub/worker pattern** is the recommended way to use wt for orchestrating multiple AI coding sessions.

## Overview

```
┌─────────────────────────────────────────────────────────────┐
│                          Hub                                 │
│  ┌───────────────────────────────────────────────────────┐  │
│  │  You (or Claude) orchestrating:                       │  │
│  │  - Groom beads                                        │  │
│  │  - Spawn workers                                      │  │
│  │  - Monitor progress                                   │  │
│  │  - Review PRs                                         │  │
│  │  - Handle blockers                                    │  │
│  └───────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
         │              │              │
         ▼              ▼              ▼
    ┌─────────┐   ┌─────────┐   ┌─────────┐
    │ Worker  │   │ Worker  │   │ Worker  │
    │ toast   │   │ shadow  │   │ obsidian│
    └─────────┘   └─────────┘   └─────────┘
```

## The Hub

The hub is your command center. From here you:

1. **See available work** - `wt ready` shows beads with no blockers
2. **Spawn workers** - `wt new <bead>` creates isolated sessions
3. **Monitor progress** - `wt watch` shows live status
4. **Handle issues** - Jump to blocked workers, provide guidance
5. **Review completions** - Check PRs, merge work

### Setting Up a Hub

Create a dedicated hub session:

```bash
wt hub
```

Or work from any terminal - the hub is conceptual, not required.

### Hub with Watch

Start hub with live dashboard:

```bash
wt hub --watch
```

This creates a split pane showing all worker statuses.

## Workflow Example

### 1. Start Your Day

```bash
# Check what's ready to work on
wt ready
# myproject-abc  Add user authentication
# myproject-def  Fix login validation
# myproject-ghi  Update error messages

# See overall project status
bd stats
```

### 2. Spawn Workers

```bash
# Spawn sessions for ready beads
wt new myproject-abc
# → Spawned session 'toast' for bead myproject-abc

wt new myproject-def
# → Spawned session 'shadow' for bead myproject-def
```

### 3. Monitor Progress

```bash
# See all sessions
wt

# Or watch live
wt watch
```

Dashboard shows:

- Session name and bead
- Status (working/idle/blocked/error)
- Last activity
- Bead title

### 4. Handle Blockers

If a worker signals blocked:

```bash
# Jump to the blocked session
wt shadow

# Inside: see what's wrong
wt status
# Status: blocked
# Message: Need API credentials for external service

# Fix the issue, then signal ready
wt signal working "Got credentials, continuing"
```

### 5. Review Completions

When workers signal ready:

```bash
# Check the PR
gh pr view <pr-number>

# Or use seance to ask questions
wt seance toast -p "What approach did you take for auth?"
```

### 6. Clean Up

```bash
# After PR is merged
wt close toast
wt close shadow

# Or close all completed sessions
wt close toast shadow obsidian
```

## Hub Session Commands

From the hub:

| Command | Purpose |
|---------|---------|
| `wt ready` | Show available work |
| `wt new <bead>` | Spawn worker |
| `wt` | List all sessions |
| `wt watch` | Live dashboard |
| `wt <name>` | Jump to session |
| `wt close <name>` | Complete and cleanup |
| `wt kill <name>` | Force kill |
| `wt seance <name>` | Query past session |

## Hub Handoff

When your Claude session is getting long, hand off to a fresh instance:

```bash
wt handoff -c -m "Continue reviewing PRs for auth work"
```

This:

1. Collects current state (sessions, ready beads)
2. Creates handoff context
3. Starts fresh Claude with the context

## Best Practices

### 1. Keep Workers Focused

Each worker handles one bead. Don't give workers additional tasks - create new beads instead.

### 2. Use Watch Dashboard

The `wt watch` dashboard helps you notice:

- Workers that went idle (might need help)
- Workers that errored (need intervention)
- Workers that completed (ready for review)

### 3. Handle Blockers Quickly

Blocked workers are wasted workers. When you see a blocker:

1. Jump to the session
2. Understand the issue
3. Either fix it or create a dependency bead
4. Signal the worker to continue

### 4. Review Before Closing

Before `wt close`:

- Check the PR looks good
- Verify tests passed
- Ensure the bead is truly complete

### 5. Clean Up Regularly

Don't let finished sessions accumulate:

```bash
# Check for sessions marked ready
wt | grep "Ready"

# Close completed ones
wt close toast shadow
```

## Parallel Processing

Spawn multiple workers for independent beads:

```bash
# Check dependencies
bd blocked  # Nothing? Good.

# Spawn all ready work
wt ready | while read bead; do
  wt new $bead
done
```

Or use auto mode:

```bash
wt auto --max=5
```
