# Beads Integration

wt integrates with [beads](https://github.com/steveyegge/beads), a git-native issue tracking system. Each wt session works on exactly one bead.

## What are Beads?

Beads is a lightweight issue tracker that stores issues directly in your git repository. Issues are:

- Tracked in `.beads/` directory
- Version controlled with your code
- Synced via git push/pull
- No external service required

## How wt Uses Beads

### One Bead = One Session

When you create a session, you specify a bead:

```bash
wt new myproject-abc123
```

The session is now bound to that bead. All work in the session relates to that single task.

### Bead Lifecycle

| Stage | wt Action | Bead Status |
|-------|-----------|-------------|
| Create session | `wt new` | `open` → `in_progress` |
| Complete work | `wt done` | `in_progress` → `awaiting_review` |
| Close session | `wt close` | (depends on merge mode) |

### Shared Beads Directory

Each worker session has access to the main repo's beads:

```bash
# Inside a worker session:
echo $BEADS_DIR
# /Users/you/myproject/.beads

# All bd commands work normally:
bd list
bd show myproject-abc123
```

## Finding Work

### Ready Beads

See beads that are ready to work on (no blockers):

```bash
# From hub
wt ready

# Or for a specific project
wt ready myproject

# Using beads directly
bd ready
```

### Creating Beads

Create a new bead from wt:

```bash
wt create myproject "Add user authentication"
```

Or use beads directly:

```bash
bd create --title="Add user authentication" --type=feature
```

## Viewing Beads

### List All Beads

```bash
wt beads myproject
# Or
bd list
```

### Show Bead Details

```bash
bd show myproject-abc123
```

## Dependencies

Beads support dependencies - one issue blocking another:

```bash
# Add dependency
bd dep add myproject-def myproject-abc  # def depends on abc

# See blocked issues
bd blocked
```

wt respects these:

- `wt ready` only shows unblocked beads
- The hub can focus on what's actually workable

## Best Practices

1. **One bead per logical change** - Keep beads focused
2. **Use dependencies** - If task B needs task A, make it explicit
3. **Update bead status** - Let wt handle status transitions
4. **Sync regularly** - Run `bd sync` to share bead state with team

## Example Workflow

```bash
# Hub: Check what's ready
wt ready
# myproject-abc123  Add auth flow
# myproject-def456  Fix login bug

# Spawn workers
wt new myproject-abc123
wt new myproject-def456

# Worker (toast): Complete work
wt done
# → Bead marked as awaiting_review
# → PR created

# Hub: Review PR, merge
# → Bead auto-closed on merge

# Hub: Clean up
wt close toast
```
