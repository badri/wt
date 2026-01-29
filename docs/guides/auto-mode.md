# Auto Mode

**Auto mode** enables autonomous sequential processing of beads within an epic. wt creates a single worktree, processes each bead in order, and reports results.

## Overview

Auto mode requires an epic. It processes the epic's child beads sequentially in one worktree, killing and restarting the Claude process between beads to prevent context rot.

Use auto mode when you want to:

- Process an epic's beads overnight without intervention
- Run a batch of related tasks sequentially
- Avoid manual session management for multi-bead epics

## Basic Usage

```bash
# Process all beads in an epic
wt auto --epic <epic-id>

# Preview what would be processed
wt auto --epic <epic-id> --dry-run

# Check status of a running auto session
wt auto --check
```

## Options

| Flag | Description |
|------|-------------|
| `--epic <id>` | **(Required)** Epic ID to process |
| `--project <name>` | Filter to specific project |
| `--timeout <minutes>` | Timeout per bead (default: 30min) |
| `--merge-mode <mode>` | Override merge mode for this run |
| `--dry-run` | Preview without executing |
| `--check` | Check status of running auto |
| `--stop` | Gracefully stop after current bead |
| `--pause-on-failure` | Stop and preserve worktree if a bead fails |
| `--skip-audit` | Bypass the implicit audit check |
| `--resume` | Resume after failure or pause |
| `--abort` | Abort and clean up after failure |
| `--force` | Override lock (risky) |

## How It Works

1. **Audit**: Validates the epic — checks beads have descriptions, no external blockers
2. **Worktree**: Creates a single worktree and tmux session for the entire epic
3. **Process**: Sends the first bead's prompt to the Claude session
4. **Complete**: After each bead completes, captures commit info and marks it done
5. **Refresh**: Kills the Claude process (not the session) to get fresh context
6. **Next**: Sends the next bead's prompt into the same tmux session
7. **Repeat**: Continues until all beads are processed
8. **Merge**: Merges the worktree branch according to the merge mode

All beads accumulate commits in the same worktree branch. The merge with the parent branch happens once at the end.

## Epic Setup

Before running auto, set up an epic with linked children:

```bash
# Create the epic
bd create --title="Documentation batch" --type=epic

# Create child beads
bd create --title="Update API docs" --type=task
bd create --title="Add examples" --type=task

# Link children to epic
bd dep add wt-child1 wt-epic-id
bd dep add wt-child2 wt-epic-id
```

## Examples

### Process an Epic

```bash
wt auto --epic wt-doc-batch
```

### Dry Run

```bash
wt auto --epic wt-doc-batch --dry-run
```

Shows what would be processed:

```
=== Dry Run ===
Would process 3 bead(s) in epic wt-doc-batch:
  1. wt-abc: Update API docs
  2. wt-def: Add examples
  3. wt-ghi: Fix broken links

Would create single worktree for sequential processing.
Worker signals completion via: wt signal bead-done "<summary>"
```

### Set Timeout

```bash
wt auto --epic wt-doc-batch --timeout 60
```

Each bead gets up to 60 minutes before being considered failed.

### Pause on Failure

```bash
wt auto --epic wt-doc-batch --pause-on-failure
```

Stops processing and preserves the worktree if any bead fails, so you can inspect and fix.

### Resume After Failure

```bash
wt auto --resume
```

Picks up where it left off, retrying failed beads.

### Abort a Run

```bash
wt auto --abort
```

Cleans up worktree and state from a failed or paused run.

## Managing Auto Mode

### Check Status

```bash
wt auto --check
```

Shows:

- Current epic and progress (e.g., 2/5 beads completed)
- Which bead is currently being processed
- Any failed beads

### Stop Processing

```bash
wt auto --stop
```

- Current bead continues to completion
- No new beads are started
- State is preserved for `--resume`

## Completion

After all beads are processed:

```
=== All 3 bead(s) processed ===
  Completed: 3
✓ Epic wt-doc-batch closed
```

If some beads failed:

```
=== All 3 bead(s) processed ===
  Completed: 2
  Failed: 1
    - wt-ghi: timeout

✗ Epic wt-doc-batch NOT closed due to failed beads
  Fix failures and run 'wt auto --resume --epic wt-doc-batch' to retry
  Or run 'wt auto --abort --epic wt-doc-batch' to clean up
```

## Best Practices

### 1. Audit Before Running

Auto mode runs an implicit audit, but you can check manually:

```bash
wt auto --epic wt-doc-batch --dry-run
```

### 2. Groom Beads Well

Each bead gets a fresh Claude context. Good descriptions make the difference:

```bash
bd show wt-abc   # Ensure description is clear and actionable
```

### 3. Set Reasonable Timeouts

Prevent runaway sessions:

```bash
wt auto --epic wt-batch --timeout 45
```

### 4. Link Dependencies Properly

Auto mode only finds children linked via `bd dep add`. Notes-only references are not detected:

```bash
bd dep add <child-id> <epic-id>
```

### 5. Use Pause on Failure for Important Work

```bash
wt auto --epic wt-release --pause-on-failure
```

This lets you inspect failures before deciding to continue or abort.

## Limitations

- **Sequential only**: Beads are processed one at a time in a single worktree
- **Epic required**: Cannot process arbitrary ready beads without an epic
- **No intervention**: Auto mode doesn't handle interactive prompts or stuck workers
- **Single lock**: Only one auto run at a time across all projects
