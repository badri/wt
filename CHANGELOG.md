# Changelog

All notable changes to wt will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.4.0] - 2026-01-21

### Added
- `wt task <description>` - Lightweight task sessions for transient work without beads
- `wt bead create` - Create beads from within task sessions when work needs tracking
- `wt prime --hook` - Capture session ID from Claude's SessionStart hook for seance
- Session name included in worker commit messages via `WT_SESSION` environment variable
- Merge conflict detection and auto-rebase in `wt done`
- Workers self-merge for direct mode projects (no PR creation)
- `wt seance --spawn` - Resume past sessions in new tmux session

### Fixed
- Namepool now correctly skips already-used session names
- Worker startup race conditions - uses `NewSessionWithCommand` pattern
- Session ID capture for seance now works reliably via `wt prime --hook`
- `wt new` now finds project before validating bead (works from hub)
- Project config always loaded in `wt new` (fixes merge mode defaulting to pr-review)
- Worktree path collisions resolved by using bead ID for paths
- `wt close` now context-aware (works from hub or worker)
- `WT_HUB` env var properly available to hub shell
- `--dangerously-skip-permissions` used correctly for seance resume and handoff

### Changed
- Handoff system simplified from bead-based to file-based
- Documentation rewritten with conversation-first approach
- Prompt for merge mode when registering new projects

## [0.3.0] - 2026-01-20

### Added
- MkDocs Material documentation site with GitHub Pages deployment
- `--json` flag for machine-readable output on list commands
- Bead title display in `wt list`, `wt watch`, and `wt pick`
- Audio alerts for macOS notifications on session completion
- Detailed help for all commands with `-h/--help` support
- Bubbletea-based TUI for `wt watch` with detail card and keyboard navigation
- Bubbletea table components for `wt list`, `wt projects`, `wt events`, `wt seance`

### Fixed
- `wt watch` now runs TUI directly when not in a wt worker session
- `wt watch` persists and accessible from worker sessions via popup
- Cross-project bead titles display correctly (uses session's BeadsDir)
- Initial prompt now correctly sent to worker sessions
- Improved prompt delivery and watch pane control in hub

### Changed
- `wt list` columns: Name, Status, Title, Project (shows title instead of activity)
- `wt watch` list shows title instead of bead ID
- `wt pick` shows title in session picker
- Improved CLI consistency for project commands
- All listing commands use styled table output

## [0.2.0] - 2026-01-19

### Added
- `wt handoff` - Handoff hub session to fresh Claude instance with context preservation
- `wt handoff -m "notes"` - Include custom message in handoff
- `wt handoff -c` - Auto-collect state (sessions, ready beads, in-progress work)
- `wt handoff --dry-run` - Preview what would be collected
- `wt prime` - Inject startup context on new session (for hook integration)
- `wt prime --quiet` - Suppress non-essential output
- `wt prime --no-bd-prime` - Skip running bd prime
- Hub Handoff bead for persistent context storage
- Handoff marker file for post-handoff detection
- New bead package functions: List, Search, Create, UpdateDescription

## [0.1.0] - 2026-01-19

### Added

#### Core Commands
- `wt` / `wt list` - List active sessions with status indicators
- `wt new <bead>` - Create new session (worktree + tmux + Claude)
- `wt <name>` - Switch to session by name or bead ID
- `wt kill <name>` - Kill session, keep bead open
- `wt close <name>` - Close session and bead
- `wt done` - Complete work, merge via configured mode
- `wt status` - Show current session status
- `wt abandon` - Abandon session without merging

#### Project Management
- `wt projects` - List registered projects
- `wt project add <name> <path>` - Register a project
- `wt project config <name>` - Edit project configuration
- `wt project remove <name>` - Unregister a project
- `wt ready [project]` - Show ready beads (optional project filter)
- `wt create <project> <title>` - Create bead in specific project
- `wt beads <project>` - List beads for a project

#### Monitoring & Events
- `wt watch` - Live dashboard of all sessions
- `wt events` - Show wt events log
- `wt events --tail` - Follow events in real-time
- `wt events --new` - Show new events since last read

#### Seance (Past Sessions)
- `wt seance` - List past sessions with Claude context
- `wt seance <name>` - Resume Claude conversation
- `wt seance <name> -p "prompt"` - One-shot query to past session
- Fuzzy matching for session names

#### Automation
- `wt auto` - Autonomous batch processing of ready beads
- `wt auto --dry-run` - Preview what would be processed
- `wt auto --check` - Check auto mode status
- `wt auto --stop` - Stop auto mode

#### Diagnostics
- `wt doctor` - Check wt setup and diagnose issues

#### Shell Integration
- `wt pick` - Interactive session picker with fzf
- `wt keys` - Output tmux keybinding configuration
- `wt completion bash` - Bash completion script
- `wt completion zsh` - Zsh completion script
- `wt completion fish` - Fish completion script
- `wt version` - Show version info

#### Themed Namepools
Six themes with 20 names each, assigned per-project:
- **kungfu-panda**: po, tigress, shifu, oogway...
- **toy-story**: woody, buzz, jessie, rex...
- **ghibli**: totoro, chihiro, haku, calcifer...
- **star-wars**: luke, leia, han, vader, yoda...
- **dune**: paul, leto, stilgar, chani...
- **matrix**: neo, trinity, morpheus, oracle...

#### Distribution
- npm package: `@worktree/wt`
- Go install: `go install github.com/badri/wt/cmd/wt@latest`
- Makefile with version injection

#### Test Environment Support
- Port offset allocation for parallel test environments
- Setup/teardown hooks per project
- Health check waiting

#### Merge Modes
- `direct` - Merge directly to default branch
- `pr-auto` - Create PR with auto-merge enabled
- `pr-review` - Create PR for manual review
