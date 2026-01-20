# Project Guidelines

## Code Quality

1. **File Size Limits**: No Go files should exceed 1000 lines. If a file grows beyond this limit, split it into multiple modules.

2. **Testing Requirements**:
   - Run tests after every code change
   - Fix any failing tests before proceeding
   - Run: `go test ./...`

3. **Integration Testing**:
   - Run integration tests after major changes (once implemented)
   - Integration tests should verify tmux, git worktree, and beads integration

4. **When to Add Tests**:
   - New public functions/APIs: Add unit tests
   - Pure functions (no external deps): Always add unit tests
   - Functions calling external commands (bd, tmux, git): Add tests that skip if command unavailable
   - UI/display changes: Generally not unit tested unless there's complex logic
   - Bug fixes: Add regression test if feasible

5. **When to Update Docs** (docs/ directory):
   - New commands or flags: Update relevant command reference
   - New concepts: Add to concepts/ section
   - Changed user-facing behavior: Update affected pages
   - Internal refactors/plumbing: No docs update needed
   - Bug fixes: No docs update unless it changes documented behavior

6. **When to Update Skills**:
   - New commands users should know about: Update skill description
   - Changed command syntax: Update skill examples
   - Internal improvements: No skill update needed

7. **When Cutting a Release**:
   - Review commits since last release: `git log --oneline --since="<last-release-date>" | grep -v "bd sync"`
   - Update CHANGELOG.md based on actual commits, not memory
   - Move [Unreleased] items to new version section with date
   - Follow Keep a Changelog format (Added, Changed, Fixed, Removed)
   - Tag with semantic version (vX.Y.Z)
   - Push tag: `git tag vX.Y.Z && git push origin vX.Y.Z`
