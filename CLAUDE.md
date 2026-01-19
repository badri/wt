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
