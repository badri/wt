# Hub Workflow

The **hub** is where you orchestrate your AI coding work. It's a conversation with Claude where you describe what needs to be done, and Claude manages the workers.

## The Key Insight

**You don't manually run wt commands.** Instead, you have a conversation with Claude in your hub, and Claude handles the commands for you:

```
You: "I need to fix that login bug. Can you spawn a worker for it?"

Claude: I'll create a bead and spawn a worker.
> bd create --title="Fix login bug" --type=bug
> wt new myproject-abc

Session 'toast' is ready and working on it.
```

This is the natural way to use wt—through conversation, not memorizing commands.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                          Hub                                 │
│  ┌───────────────────────────────────────────────────────┐  │
│  │  You + Claude orchestrating:                          │  │
│  │  - Plan what to work on                               │  │
│  │  - Spawn workers for tasks                            │  │
│  │  - Monitor progress                                   │  │
│  │  - Review completed work                              │  │
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

## What the Hub Does

The hub is your command center where you:

1. **Plan your work** - Review ready beads, prioritize, create new work
2. **Spawn workers** - Ask Claude to create sessions for tasks
3. **Monitor progress** - Check on workers, see the live dashboard
4. **Handle issues** - Jump to blocked workers, provide guidance
5. **Review completions** - Check PRs, decide on merge strategies

### Starting Your Hub

Create a dedicated hub session:

```bash
wt hub
```

Or simply start Claude in any terminal—the hub is conceptual, not a strict requirement. The `wt hub` command just gives you a dedicated tmux session for orchestration.

### Hub with Dashboard

Start hub with live monitoring:

```bash
wt hub --watch
```

This creates a split pane showing all worker statuses in real-time.

## A Typical Day (Conversation Style)

Here's how a day looks when working through your hub:

### Morning: Check What's Ready

```
You: "What work is available today?"

Claude: Let me check.
> wt ready

Three beads ready:
- myproject-abc: Add user authentication
- myproject-def: Fix login validation
- myproject-ghi: Update error messages

> bd stats

Project status: 3 open, 2 in progress, 12 closed this week.
```

### Start Some Workers

```
You: "Let's work on the auth and validation fixes in parallel."

Claude: Spawning workers for both.
> wt new myproject-abc  # → toast
> wt new myproject-def  # → shadow

Two workers running:
- toast: Working on authentication
- shadow: Working on login validation
```

### Check Progress

```
You: "How are they doing?"

Claude:
> wt

┌─ Active Sessions ─────────────────────────────────────────────┐
│ 🟢 toast    myproject-abc   Working   Add user authentication │
│ 🟢 shadow   myproject-def   Working   Fix login validation    │
└───────────────────────────────────────────────────────────────┘

Both are actively working. Toast has made 3 commits, shadow is still
on its first approach.
```

### Handle a Blocker

```
You: "Shadow signaled blocked. What's wrong?"

Claude: Let me check.
> wt shadow
> wt status

Shadow is blocked: "Need database migration for new validation field"

You: "Tell it to create the migration itself—it has full permissions."

Claude:
> wt signal working "You can create the migration."

Shadow is unblocked and continuing.
```

### Review Completed Work

```
You: "Toast says it's done. Show me what it did."

Claude:
> gh pr view 45

PR #45: Add user authentication
- 8 files changed, +342 -12
- Tests passing

Want me to summarize the approach?
```

### End of Day Cleanup

```
You: "PR #45 was merged. Clean up toast."

Claude:
> wt close toast

Toast closed. Shadow is still working.
```

For more detailed scenarios, see [Sample Workflows](sample-workflows.md).

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

### Keep Workers Focused

Each worker handles one bead. If you find yourself wanting to give a worker additional work, create a new bead instead:

```
You: "Toast is almost done with auth. Can it also fix the session timeout?"

Claude: I'd recommend creating a separate bead for the timeout fix. That way
we keep clear boundaries and can track it independently.
> bd create --title="Fix session timeout" --type=bug

Want me to spawn another worker for it?
```

### Monitor Regularly

Ask Claude to check on things:

```
You: "Check on all the workers."

Claude:
> wt watch

- toast: Working steadily, 4 commits in the last hour
- shadow: Idle for 10 minutes—might be stuck
- obsidian: Just signaled ready, PR #46 created
```

### Handle Blockers Quickly

Blocked workers are wasted workers:

```
You: "Any workers blocked?"

Claude:
> wt | grep -i blocked

Shadow is blocked: "Need staging credentials"

You: "The credentials are in the team vault under 'Staging DB'."

Claude: I'll pass that along.
> wt signal working "Credentials in team vault under 'Staging DB'"
```

### Review Before Closing

Have Claude help you review:

```
You: "Toast is done. Before I close it, show me the changes and run tests."

Claude:
> gh pr view 45 --comments
> cd ~/worktrees/toast && npm test

PR looks good: 8 files, all tests passing. Ready to close?
```

### Clean Up Regularly

```
You: "Any sessions I should clean up?"

Claude:
> wt

You have 3 sessions where PRs were merged:
- toast (PR #45)
- shadow (PR #46)
- obsidian (PR #47)

Should I close all of them?

You: "Yes, clean them up."

Claude:
> wt close toast shadow obsidian

All sessions closed.
```

## Parallel Processing

Ask Claude to maximize throughput:

```
You: "Spawn workers for all ready beads."

Claude:
> wt ready
5 beads ready.

> wt new myproject-abc  # → toast
> wt new myproject-def  # → shadow
> wt new myproject-ghi  # → obsidian
> wt new myproject-jkl  # → quartz
> wt new myproject-mno  # → jasper

5 workers running in parallel. Use `wt watch` to monitor them.
```

Or let wt handle it automatically:

```
You: "Process all ready beads automatically."

Claude:
> wt auto

Auto mode started. Will process 5 beads sequentially, creating
PRs as each completes.
```

## Next Steps

- [Sample Workflows](sample-workflows.md) - More detailed scenarios
- [Auto Mode](auto-mode.md) - Hands-off batch processing
- [Seance](seance.md) - Querying past sessions
