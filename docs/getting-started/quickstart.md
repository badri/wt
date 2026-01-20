# Quick Start

This guide will get you up and running with wt in 5 minutes.

## 1. Register a Project

First, register your project with wt:

```bash
wt project add myproject ~/code/myproject
```

This tells wt where your project lives and enables session management for it.

## 2. Find Work

See what beads are ready to work on:

```bash
wt ready
# Or for a specific project:
wt ready myproject
```

## 3. Spawn a Session

Create a new session for a bead:

```bash
wt new myproject-abc123
```

This will:

1. Create a git worktree in `~/worktrees/<session-name>/`
2. Create a new branch from the bead ID
3. Start a tmux session
4. Launch Claude Code in the session
5. Mark the bead as `in_progress`

You'll see output like:

```
Spawned session 'toast' for bead myproject-abc123
  Worktree: ~/worktrees/toast
  Branch: myproject-abc123
  Port offset: 1
```

## 4. Work in the Session

You're now inside a Claude Code session. Work on your task as normal.

Check your session status anytime:

```bash
wt status
```

## 5. List All Sessions

Open another terminal to see all active sessions:

```bash
wt
```

Output:
```
┌─ Active Sessions ────────────────────────────────────────┐
│ 🟢 toast    myproject-abc   Working   Add auth flow     │
└──────────────────────────────────────────────────────────┘
```

## 6. Switch Between Sessions

Jump to a different session:

```bash
wt toast
# or
wt shadow
```

## 7. Complete Your Work

When you're done with the task:

```bash
wt done
```

This will:

1. Commit your changes
2. Push to the remote
3. Create a PR (depending on merge mode)
4. Update the bead status

## 8. Close the Session

Clean up the session and worktree:

```bash
wt close toast
```

Or from inside the session:

```bash
wt close
```

## What's Next?

- [Hub Workflow](../guides/hub-workflow.md) - Learn the hub/worker pattern
- [Commands Reference](../commands/hub.md) - Full command reference
- [Configuration](../reference/configuration.md) - Customize wt behavior
