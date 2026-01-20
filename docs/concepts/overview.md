# Concepts Overview

wt is built around a few key concepts that work together to provide isolated, persistent agent sessions.

## The wt Model

```
┌──────────────────────────────────────────────────────────────┐
│                         Hub Session                          │
│  ┌────────────────────────────────────────────────────────┐ │
│  │  Orchestrator (you or Claude) grooms beads, spawns     │ │
│  │  workers, monitors progress, reviews PRs               │ │
│  └────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────┘
         │                    │                    │
         ▼                    ▼                    ▼
┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐
│ Worker: toast   │  │ Worker: shadow  │  │ Worker: obsidian│
│ ┌─────────────┐ │  │ ┌─────────────┐ │  │ ┌─────────────┐ │
│ │ Bead: abc   │ │  │ │ Bead: def   │ │  │ │ Bead: ghi   │ │
│ │ Worktree    │ │  │ │ Worktree    │ │  │ │ Worktree    │ │
│ │ Tmux session│ │  │ │ Tmux session│ │  │ │ Tmux session│ │
│ │ Claude Code │ │  │ │ Claude Code │ │  │ │ Claude Code │ │
│ └─────────────┘ │  │ └─────────────┘ │  │ └─────────────┘ │
└─────────────────┘  └─────────────────┘  └─────────────────┘
```

## Core Principles

### One Bead = One Session = One Worktree

This is the fundamental principle of wt:

- Each **bead** (task) gets exactly one **session**
- Each **session** has exactly one **worktree**
- No overlap, no shared state, complete isolation

### Sessions Persist

Sessions don't auto-cleanup or expire. They persist until you explicitly close them with `wt close`. This means:

- Disconnect from your terminal? Session keeps running.
- Close your laptop? Session survives.
- Come back tomorrow? Pick up where you left off.

### Hub/Worker Pattern

wt encourages a hub/worker pattern:

- **Hub**: Where you groom beads, spawn workers, monitor progress
- **Workers**: Isolated sessions where Claude works on specific tasks

## Key Components

| Component | Purpose |
|-----------|---------|
| [Sessions](sessions.md) | Container for agent work: tmux + worktree + Claude |
| [Worktrees](worktrees.md) | Git's native isolation mechanism |
| [Beads](beads.md) | Git-native task tracking |
| [Test Environments](test-environments.md) | Per-session isolated services |
| [Merge Modes](merge-modes.md) | How completed work gets merged |

## Typical Workflow

1. **Groom beads** - Use `bd ready` to see available work
2. **Spawn workers** - `wt new <bead-id>` creates isolated sessions
3. **Monitor progress** - `wt watch` shows live status
4. **Review work** - Check PRs or use `wt seance` to query past sessions
5. **Complete work** - `wt done` commits and creates PRs
6. **Cleanup** - `wt close` removes sessions and worktrees
