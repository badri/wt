# CLI Reference

**Note:** You typically don't run these commands directly. Instead, you describe what you want to Claude in your hub session, and Claude runs the appropriate commands for you.

```
You: "Spawn a worker for the auth task"
Claude: [runs wt new myproject-abc]
```

This reference documents what's happening behind the scenes.

## When to Use Commands Directly

Most users never need to run wt commands manually. However, you might use them for:

- **Debugging** — Understanding what Claude is doing
- **Automation** — Scripting wt into CI/CD or shell scripts
- **Quick actions** — Power users who prefer typing commands

## Command Categories

### Hub Commands

Commands run from your orchestration session (the hub):

- `wt` / `wt list` — List active sessions
- `wt new <bead>` — Spawn a new worker
- `wt <name>` — Switch to a session
- `wt watch` — Live dashboard
- `wt close <name>` — Complete work and clean up
- `wt ready` — Show available beads
- `wt hub` — Create/attach to hub session
- `wt auto` — Autonomous batch processing

See [Hub Commands](hub.md) for full details.

### Worker Commands

Commands run from inside a worker session:

- `wt status` — Show current session info
- `wt done` — Complete work and create PR
- `wt signal <status>` — Update session status
- `wt abandon` — Discard changes and close

See [Worker Commands](worker.md) for full details.

### Utility Commands

Diagnostic and helper commands:

- `wt doctor` — Diagnose setup issues
- `wt events` — View event log
- `wt completion` — Shell completions
- `wt handoff` — Hand off hub to fresh Claude

See [Utility Commands](utilities.md) for full details.

### Configuration

- `wt config` — Manage wt configuration
- `wt project` — Manage project registrations

See [Configuration Commands](config.md) for full details.
