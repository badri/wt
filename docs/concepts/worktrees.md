# Worktrees

wt uses **git worktrees** to provide code isolation between sessions. This is Git's native mechanism for having multiple working directories from the same repository.

## What is a Git Worktree?

A worktree is a linked copy of your repository that shares the same `.git` directory but has its own working directory and branch. This means:

- Each worktree can be on a different branch
- Changes in one worktree don't affect others
- All worktrees share the same commit history
- Pushing from any worktree updates the same remote

## Why Worktrees?

### Isolation Without Cloning

Traditional approaches to parallel work:

| Approach | Pros | Cons |
|----------|------|------|
| Branch switching | Simple | Context switching overhead |
| Multiple clones | Full isolation | Disk space, sync issues |
| **Worktrees** | Isolation + shared history | Best of both |

### Perfect for AI Agents

When running multiple Claude sessions:

- Each session needs its own files to edit
- Sessions shouldn't step on each other's changes
- But they should share the same git history
- Worktrees provide exactly this

## How wt Uses Worktrees

When you create a session:

```bash
wt new myproject-abc123
```

wt runs:

```bash
git worktree add ~/worktrees/toast -b myproject-abc123
```

This creates:

```
~/worktrees/toast/           # New worktree directory
├── src/                     # Full copy of source
├── tests/
└── ...                      # All project files

# In the main repo's .git:
.git/worktrees/toast/        # Worktree metadata
```

## Worktree Location

By default, worktrees are created in `~/worktrees/`. You can change this:

```bash
wt config set worktree_root /custom/path
```

## Branch Strategy

Each worktree gets its own branch, named after the bead:

```
main
├── myproject-abc123  (toast worktree)
├── myproject-def456  (shadow worktree)
└── myproject-ghi789  (obsidian worktree)
```

## Listing Worktrees

See all git worktrees for a repo:

```bash
git worktree list
# /Users/you/myproject         abc1234 [main]
# /Users/you/worktrees/toast   def5678 [myproject-abc123]
# /Users/you/worktrees/shadow  ghi9012 [myproject-def456]
```

## Cleanup

When you close a session, wt removes the worktree:

```bash
wt close toast
# Removes ~/worktrees/toast/
# Runs: git worktree remove toast
```

## Manual Worktree Management

If you need to manually manage worktrees:

```bash
# Add a worktree
git worktree add ../my-worktree -b my-branch

# List worktrees
git worktree list

# Remove a worktree
git worktree remove ../my-worktree

# Prune stale worktree references
git worktree prune
```

## Best Practices

1. **Let wt manage worktrees** - Don't manually create worktrees for wt sessions
2. **Use `wt close`** - Ensures proper cleanup of worktrees and session state
3. **Check `git worktree list`** - If things get out of sync, this shows the true state
4. **Run `git worktree prune`** - Cleans up if worktrees were manually deleted
