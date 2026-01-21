# Sample Workflows

Practical scenarios showing how to use wt for common development patterns. These examples focus on what you'll actually say to Claude in your hub session.

## Overview

Most interaction with wt happens through conversation with Claude in your hub session. The hub is your command center—you describe what you want, and Claude orchestrates the workers.

```
You → Hub (Claude) → Workers
```

---

## Scenario 1: Safe Changes (PR Review Mode)

**When to use:** Production code, team projects, anything that needs review before merging.

### Starting the Session

Open your hub and ask Claude to work on something:

```
You: "I have a bug where users can't reset their passwords. Can you spawn a
worker to fix it?"

Claude: I'll create a bead for this and spawn a worker.
> bd create --title="Fix password reset bug" --type=bug --priority=2
> wt new myproject-abc

Session 'toast' spawned for the password reset fix. I'll switch you there.
```

### Monitoring Progress

Check in from the hub:

```
You: "How's toast doing?"

Claude: Let me check.
> wt
toast is still working on the password reset fix. Last activity 2 minutes ago.
```

### Reviewing the Work

When the worker signals completion:

```
You: "Toast says it's done. What did it change?"

Claude: Let me check the PR.
> gh pr view <number>

The worker made these changes:
- Fixed token expiration in reset_password.py
- Added retry logic for email sending
- Added regression test

Want me to review the code more closely?
```

### Completing the Work

After you (or a teammate) review and merge the PR:

```
You: "The PR got merged. Clean up toast."

Claude:
> wt close toast

Session closed. The password reset fix is now in main.
```

---

## Scenario 2: YOLO Mode (Direct Merge)

**When to use:** Personal projects, prototypes, experiments, or when you fully trust the AI's work.

### Configuration

First, configure your project for direct merge:

```
You: "Configure my-prototype to use direct merge mode."

Claude:
> wt project config my-prototype

I've set merge_mode to "direct". Changes will push straight to main
without PRs.
```

### The Workflow

```
You: "Add a dark mode toggle to the settings page."

Claude: Creating bead and spawning worker.
> bd create --title="Add dark mode toggle" --type=feature
> wt new my-prototype-xyz

Session 'shadow' ready. Switching you there now.
```

When shadow completes:

```
You: "Shadow is done. Merge it."

Claude:
> wt done
> wt close shadow

Done. Dark mode toggle is now live on main.
```

No PR, no review—straight to main.

---

## Scenario 3: Grooming Session

**When to use:** Starting your day, planning a sprint, organizing work.

### Reviewing What's Available

```
You: "What work is ready to tackle today?"

Claude:
> wt ready

Ready beads (no blockers):
- myproject-abc: Fix login validation
- myproject-def: Update error messages
- myproject-ghi: Add export feature

> bd blocked

Blocked beads:
- myproject-jkl: API integration (blocked by myproject-ghi)
```

### Prioritizing Work

```
You: "Let's focus on the validation fix first—it's blocking QA. Then tackle
the export feature since it unblocks the API work."

Claude: Good prioritization. Shall I spawn workers for both? I can run them
in parallel since they're independent.

You: "Yes, spawn them both."

Claude:
> wt new myproject-abc  # → toast
> wt new myproject-ghi  # → shadow

Two workers running:
- toast: Fix login validation
- shadow: Add export feature

I'll monitor them. The API integration (myproject-jkl) will become ready
once shadow completes the export feature.
```

### Creating New Work

```
You: "We also need to add pagination to the users list. Create a bead for
that but don't start it yet."

Claude:
> bd create --title="Add pagination to users list" --type=feature --priority=2

Created myproject-mno. It's ready when you want to start it.
```

---

## Scenario 4: Parallel Workers

**When to use:** Multiple independent tasks, maximizing throughput, time-sensitive work.

### Spawning Multiple Workers

```
You: "I need to ship three bug fixes today. Spawn workers for all of them."

Claude:
> wt ready
Three bugs ready: myproject-123, myproject-124, myproject-125

> wt new myproject-123  # → toast
> wt new myproject-124  # → shadow
> wt new myproject-125  # → obsidian

Three workers running in parallel:
- toast: Database timeout fix
- shadow: UI rendering glitch
- obsidian: Email validation bug

You can monitor all of them with `wt watch` or I can check periodically.
```

### Live Monitoring

```
You: "Show me the live dashboard."

Claude:
> wt watch

┌─ Active Sessions ─────────────────────────────────────────────┐
│ 🟢 toast      myproject-123   Working   Database timeout fix  │
│ 🟢 shadow     myproject-124   Working   UI rendering glitch   │
│ 🟡 obsidian   myproject-125   Idle      Email validation bug  │
└───────────────────────────────────────────────────────────────┘

Obsidian is idle. Should I check on it?
```

### Handling Different Completion Times

```
You: "Shadow is done. Review its PR while the others keep working."

Claude:
> gh pr view <shadow-pr>

Shadow's changes look good:
- Fixed z-index on modal
- Added CSS transition

The other workers are still going. Should I merge shadow's PR and close it?

You: "Yes, merge and close."

Claude:
> wt close shadow

Shadow closed. Two workers still running:
- toast: 75% done based on commits
- obsidian: Still idle—may be stuck
```

---

## Scenario 5: Session Handoff

**When to use:** Long hub sessions, context getting stale, fresh start needed.

### When Your Hub Gets Long

After hours of work, your Claude session has a lot of context:

```
You: "This session is getting long. Let's hand off to a fresh instance."

Claude: I'll collect the current state and hand off.
> wt handoff -c

Collecting state:
- Active sessions: toast (working), obsidian (ready)
- Ready beads: myproject-456, myproject-789
- Recent completions: shadow (PR #42 merged)

Starting fresh Claude with this context...
```

The new Claude instance picks up with full awareness of what's happening.

### Handoff with Instructions

```
You: "Hand off but focus the new instance on the API work."

Claude:
> wt handoff -c -m "Priority: complete the API integration work. Toast is
close to done on the export feature which unblocks it."

Handoff complete. The new instance knows to prioritize API work.
```

---

## Scenario 6: Handling Blockers

**When to use:** Workers get stuck, need external input, dependencies arise.

### When a Worker Signals Blocked

```
You: "Toast signaled it's blocked. What's wrong?"

Claude: Let me check.
> wt toast
> wt status

Toast is blocked on "Need database credentials for staging environment."

Options:
1. Provide the credentials
2. Create a dependency bead for credential setup
3. Have toast work around it

You: "The credentials are in 1Password. The staging DB user is 'app_staging'
and password is in the 'Staging DB' entry."

Claude: I'll pass that to toast.
> wt signal working "Credentials provided: user is app_staging, password in
1Password 'Staging DB' entry"

Toast should be unblocked now.
```

### Creating Dependency Work

```
You: "Actually, we should set up proper secrets management. Create a bead
for that and make toast's work depend on it."

Claude:
> bd create --title="Set up secrets management" --type=task --priority=1
> bd dep add myproject-abc myproject-xyz  # toast's bead depends on new bead

Created myproject-xyz. Toast is now blocked until secrets management is
done. Want me to spawn a worker for secrets management?
```

---

## Scenario 7: End of Day Cleanup

**When to use:** Wrapping up work, reviewing progress, preparing for tomorrow.

### Reviewing the Day

```
You: "What's the status of everything?"

Claude:
> wt

Active sessions:
- toast: Ready (PR pending review)
- shadow: Working (almost done)

> bd stats

Today's progress:
- Closed: 3 beads
- In progress: 2 beads
- Ready for tomorrow: 5 beads
```

### Cleaning Up

```
You: "Toast's PR won't get reviewed until tomorrow. Kill the session but
keep the PR open."

Claude:
> wt kill toast

Toast session killed. The PR stays open for review. The bead remains in
'awaiting_review' status.

Shadow is still working. Want to let it finish or kill it too?

You: "Let shadow finish. I'll check it in the morning."

Claude: Shadow will keep running. When you return tomorrow, run `wt` in
your hub to see its status.
```

---

## Scenario 8: Auto Mode (Hands-Off Processing)

**When to use:** Batch processing, overnight runs, fully autonomous work.

### Starting Auto Mode

```
You: "Process all ready beads automatically. Don't wait for my input."

Claude:
> wt auto

Starting auto mode:
- Found 5 ready beads
- Max concurrent workers: 1 (default)
- Merge mode: pr-review

Processing myproject-abc first...

Auto mode will:
1. Spawn worker for each bead
2. Wait for completion
3. Create PR
4. Move to next bead
5. Stop when all beads processed

You can check status anytime with `wt auto --check`.
```

### Checking Auto Mode Progress

```
You: "How's auto mode doing?"

Claude:
> wt auto --check

Auto mode status:
- Processed: 3 beads
- Current: myproject-def (shadow, 45 minutes in)
- Remaining: 2 beads
- PRs created: #43, #44, #45
```

### Stopping Auto Mode

```
You: "Stop auto mode after the current bead."

Claude:
> wt auto --stop

Auto mode will stop after shadow completes myproject-def.
Remaining beads (myproject-ghi, myproject-jkl) will stay ready for later.
```

---

## Tips for Effective Hub Conversations

### Be Specific About Intent

```
# Good
"Fix the login bug in the auth module. Focus on the token validation."

# Less good
"Fix auth"
```

### Let Claude Know Your Constraints

```
"I need this done today—spawn parallel workers if possible."
"This is experimental—use direct merge mode."
"This needs careful review—make sure to create a detailed PR."
```

### Trust the Workers, Verify the Results

Workers are autonomous. Check their output:

```
"What did toast change? Show me the diff."
"Run the tests in shadow's worktree."
"Review obsidian's PR before merging."
```

### Use Seance for Post-Mortems

When something goes wrong or you're curious:

```
"Ask toast why it chose that approach."
"What was shadow stuck on before it succeeded?"
"Did obsidian consider using a different library?"
```
