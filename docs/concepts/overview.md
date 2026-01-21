# Concepts Overview

wt lets you orchestrate multiple AI coding agents through conversation. You talk to Claude in your hub session, and Claude manages the workers.

## The wt Model

```
┌──────────────────────────────────────────────────────────────┐
│                    Hub (You + Claude)                         │
│  ┌────────────────────────────────────────────────────────┐ │
│  │  You describe what you want. Claude spawns workers,    │ │
│  │  monitors progress, handles blockers, reviews PRs.     │ │
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

wt uses a hub/worker pattern:

- **Hub**: A conversation with Claude where you plan work and orchestrate workers
- **Workers**: Isolated sessions where other Claude instances work autonomously on tasks

## Key Components

| Component | Purpose |
|-----------|---------|
| [Sessions](sessions.md) | Container for agent work: tmux + worktree + Claude |
| [Worktrees](worktrees.md) | Git's native isolation mechanism |
| [Beads](beads.md) | Git-native task tracking |
| [Test Environments](test-environments.md) | Per-session isolated services |
| [Merge Modes](merge-modes.md) | How completed work gets merged |

## Typical Workflow

From your hub (a conversation with Claude):

1. **"What's ready?"** — Claude shows available beads
2. **"Spawn a worker for the auth task"** — Claude creates an isolated session
3. **"How are the workers doing?"** — Claude shows live status
4. **"Toast says it's done. What did it change?"** — Claude reviews the PR
5. **"Close toast"** — Claude completes the work and cleans up
