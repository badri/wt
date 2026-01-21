# Quick Start

This guide will get you up and running with wt in 5 minutes. wt is designed to be used conversationally through Claude—you describe what you want, and Claude handles the commands.

## Prerequisites

Before starting, ensure you have:

1. Installed wt ([Installation](installation.md))
2. Set up the wt skill for Claude Code ([Installation - Skill Setup](installation.md#claude-code-skill-setup))
3. At least one project with beads (`bd init` in your project)

## 1. Start the Hub

The hub is your command center for orchestrating work across projects.

```
You: "Start the hub"
Claude: [runs wt hub, you're now in the hub session with a watch pane]
```

You'll see a split view: Claude on the left, session monitor on the right.

## 2. Register Your Project

Tell Claude about projects you want to manage:

```
You: "Register my project at ~/code/myapp"
Claude: "I'll register that project. How should completed work be merged?"
        - direct: Merge directly to main (no PR)
        - pr-auto: Create PR, auto-merge when CI passes
        - pr-review: Create PR, wait for human review
You: "pr-review"
Claude: [runs wt project add myapp ~/code/myapp]
        [runs wt project config to set merge_mode]
       "Registered 'myapp' with pr-review merge mode"
```

## 3. Find Available Work

Ask what's ready to work on:

```
You: "What's ready?"
Claude: [runs wt ready]
       "3 beads are ready across your projects:
        - myapp-abc: Add user authentication (P1)
        - myapp-def: Fix login timeout bug (P2)
        - backend-xyz: Update API docs (P3)"
```

## 4. Spawn a Worker

Start a worker session for a specific task:

```
You: "Work on the auth task"
Claude: [runs wt new myapp-abc]
       "Spawned worker 'toast' for myapp-abc
        - Worktree: ~/worktrees/toast
        - Branch: myapp-abc
        Worker is starting on the task."
```

The worker (another Claude session) begins working autonomously in its isolated environment.

## 5. Monitor Progress

Check on your workers anytime:

```
You: "How are the workers doing?"
Claude: [runs wt list]
       "1 active worker:
        🟢 toast - myapp-abc - Working - 'Implementing OAuth flow...'"
```

Or use the watch pane (right side of hub) for real-time updates.

## 6. Check on a Specific Worker

Switch to a worker to see what it's doing:

```
You: "Switch to toast"
Claude: [runs wt toast]
       [You're now in the toast session]
```

Use `Ctrl-b d` to detach back to the hub.

## 7. Close Completed Work

When a worker signals completion:

```
You: "Close toast"
Claude: [runs wt close toast]
       "Closed session 'toast'
        - PR created: https://github.com/org/myapp/pull/42
        - Bead myapp-abc marked complete
        - Worktree cleaned up"
```

## The Flow

```
┌─────────────────────────────────────────────────────────────┐
│  HUB (You + Claude)                                         │
│                                                             │
│  You: "What's ready?"                                       │
│  You: "Spawn a worker for task X"                           │
│  You: "How's it going?"                                     │
│  You: "Close the finished worker"                           │
│                                                             │
└─────────────────────┬───────────────────────────────────────┘
                      │ spawns/monitors
                      ▼
┌─────────────────────────────────────────────────────────────┐
│  WORKERS (Claude instances)                                 │
│                                                             │
│  toast: Working on myapp-abc in ~/worktrees/toast           │
│  shadow: Working on backend-xyz in ~/worktrees/shadow       │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

## What's Next?

- [Hub Workflow](../guides/hub-workflow.md) - Deep dive into the hub/worker pattern
- [Shell Integration](shell-integration.md) - Tmux keybindings and completions
- [Configuration](../reference/configuration.md) - Customize wt behavior

## CLI Reference

For power users who want to understand what Claude runs, see [Commands Reference](../commands/). You can run these commands manually, but the conversational approach is the intended way to use wt.
