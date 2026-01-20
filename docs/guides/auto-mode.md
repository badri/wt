# Auto Mode

**Auto mode** enables autonomous batch processing of ready beads. wt spawns workers, runs them to completion, and reports results.

## Overview

Auto mode is useful when you want to:

- Process multiple independent beads overnight
- Run a batch of similar tasks
- Maximize parallelism without manual intervention

## Basic Usage

```bash
# Process all ready beads
wt auto

# Preview what would be processed
wt auto --dry-run
```

## Options

| Flag | Description |
|------|-------------|
| `--project` | Filter to specific project |
| `--max` | Maximum concurrent sessions |
| `--merge-mode` | Override merge mode for this run |
| `--timeout` | Timeout per session |
| `--dry-run` | Preview without executing |

## Examples

### Process Specific Project

```bash
wt auto --project=myproject
```

### Limit Concurrency

```bash
wt auto --max=3
```

Only 3 workers will run at once. When one finishes, the next starts.

### Set Timeout

```bash
wt auto --timeout=30m
```

Sessions that exceed 30 minutes are killed.

### Override Merge Mode

```bash
wt auto --merge-mode=direct
```

All completions push directly to main (no PRs).

### Dry Run

```bash
wt auto --dry-run
```

Shows what would be processed without actually doing it:

```
Dry run - would process:
  myproject-abc  Add auth flow
  myproject-def  Fix login bug
  myproject-ghi  Update tests

Settings:
  Max concurrent: 5
  Merge mode: pr-review
  Timeout: none
```

## Managing Auto Mode

### Check Status

```bash
wt auto --check
```

Shows:

- Running sessions
- Completed count
- Failed count
- Queue remaining

### Stop Processing

```bash
wt auto --stop
```

- Stops spawning new sessions
- Running sessions continue
- Queue is cleared

## How It Works

1. **Queue**: All ready beads are queued
2. **Spawn**: Workers spawn up to `--max` limit
3. **Monitor**: wt watches for completion/failure
4. **Cycle**: As workers finish, new ones spawn
5. **Report**: Summary shown when queue empties

## Completion Criteria

A session completes when:

- Worker signals `ready`
- `wt done` succeeds
- PR is created (if applicable)

A session fails when:

- Worker signals `error`
- Timeout exceeded
- Worker process crashes

## Results

After auto mode finishes:

```
Auto processing complete:
  Completed: 8
  Failed: 1
  Skipped: 0

Failed:
  myproject-xyz - Timeout exceeded

PRs created:
  #123 myproject-abc Add auth flow
  #124 myproject-def Fix login bug
  ...
```

## Best Practices

### 1. Use Dry Run First

Always preview what will be processed:

```bash
wt auto --dry-run
```

### 2. Start Small

Test with a small batch first:

```bash
wt auto --max=2 --project=myproject
```

### 3. Set Reasonable Timeouts

Prevent runaway sessions:

```bash
wt auto --timeout=1h
```

### 4. Check Dependencies

Auto mode only processes ready beads. Check for blockers:

```bash
bd blocked
```

If important beads are blocked, resolve dependencies first.

### 5. Monitor Progress

Keep an eye on progress:

```bash
wt auto --check
wt watch
```

### 6. Review Results

After completion, review PRs:

```bash
gh pr list --author=@me
```

## Limitations

- **No intervention**: Auto mode doesn't handle blocked workers
- **Simple tasks**: Best for straightforward, independent beads
- **No dependencies**: Beads with dependencies may not work well
- **Resource limits**: Too many concurrent sessions can exhaust resources

## Use Cases

### Batch Bug Fixes

When you have many similar bugs:

```bash
wt auto --project=myproject --max=5
```

### Documentation Updates

For documentation-heavy work:

```bash
wt auto --merge-mode=direct --project=docs
```

### Test Coverage

Adding tests to multiple modules:

```bash
wt auto --timeout=30m --max=3
```

### Overnight Processing

Start before leaving:

```bash
wt auto --project=myproject
# Check results in the morning
```
