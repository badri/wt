# Merge Modes

wt supports different strategies for merging completed work back to the main branch. Choose the mode that fits your workflow.

## Available Modes

### Direct

Push directly to the default branch without a PR.

```json
{
  "merge_mode": "direct"
}
```

**Best for:**

- Solo projects
- Prototypes and experiments
- Trusted environments

**Workflow:**

```bash
wt done
# → Commits changes
# → Pushes directly to main
# → Bead marked as closed
```

### PR Auto-Merge

Create a PR and auto-merge when CI passes.

```json
{
  "merge_mode": "pr-auto",
  "require_ci": true,
  "auto_merge_on_green": true
}
```

**Best for:**

- Solo projects with CI/CD
- Trusted automation
- Fast iteration

**Workflow:**

```bash
wt done
# → Commits changes
# → Pushes branch
# → Creates PR
# → Enables auto-merge
# → Bead marked as awaiting_review
# (PR auto-merges when CI passes)
# → Bead auto-closed on merge
```

### PR Review (Default)

Create a PR and wait for human review.

```json
{
  "merge_mode": "pr-review",
  "require_ci": true
}
```

**Best for:**

- Team projects
- Code requiring review
- Production systems

**Workflow:**

```bash
wt done
# → Commits changes
# → Pushes branch
# → Creates PR
# → Bead marked as awaiting_review

# Human reviews and merges PR
# → Bead auto-closed on merge
```

## Configuration

### Global Default

Set the default merge mode for all projects:

```bash
wt config set default_merge_mode pr-review
```

### Per-Project

Override for specific projects:

```bash
wt project config myproject
```

```json
{
  "name": "myproject",
  "merge_mode": "pr-auto",
  "require_ci": true,
  "auto_merge_on_green": true
}
```

### Per-Session Override

Override when completing work:

```bash
wt done --merge-mode=direct
wt done --merge-mode=pr-review
```

## Configuration Options

| Option | Description | Default |
|--------|-------------|---------|
| `merge_mode` | `direct`, `pr-auto`, or `pr-review` | `pr-review` |
| `require_ci` | Wait for CI to pass before merge | `true` |
| `auto_merge_on_green` | Auto-merge PRs when CI passes | `false` |
| `default_branch` | Branch to merge into | `main` |

## PR Templates

PRs created by wt include:

- Title from bead title
- Description from bead description
- Link back to bead
- Automatic labels (if configured)

Example PR:

```markdown
## Summary
Add user authentication flow

Closes: myproject-abc123

## Changes
- Added login/logout endpoints
- Implemented JWT token handling
- Added auth middleware

## Testing
- Unit tests added
- Manual testing completed
```

## CI Integration

### GitHub Actions

wt works with GitHub's auto-merge feature. Enable in your repo:

1. Settings → General → Allow auto-merge
2. Set up branch protection rules
3. Configure `pr-auto` merge mode

### Branch Protection

Recommended settings for `pr-auto`:

- Require pull request reviews: Off (or 0 reviewers)
- Require status checks to pass: On
- Require branches to be up to date: On

For `pr-review`:

- Require pull request reviews: On (1+ reviewers)
- Require status checks to pass: On

## Bead Status Transitions

| Action | direct | pr-auto | pr-review |
|--------|--------|---------|-----------|
| `wt done` | closed | awaiting_review | awaiting_review |
| CI passes | - | auto-merge triggers | - |
| PR merged | - | closed | closed |
| Manual close | closed | closed | closed |
