#!/usr/bin/env python3
"""
PreCompact hook to save context checkpoint before Claude compacts.

This hook runs before Claude runs a compact operation (manual or automatic).
It saves the current session state to a checkpoint file so context can be
recovered when the session resumes.

Add to your Claude settings:

{
  "hooks": {
    "PreCompact": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "wt checkpoint --quiet -m 'auto'"
          }
        ]
      }
    ]
  }
}

Or use this Python script for more detailed output:

{
  "hooks": {
    "PreCompact": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "/path/to/wt-pre-compact.py"
          }
        ]
      }
    ]
  }
}
"""
import json
import sys
import subprocess
import os


def main():
    # Load hook input from stdin
    try:
        input_data = json.load(sys.stdin)
    except json.JSONDecodeError:
        # Invalid input, still try to checkpoint
        input_data = {}

    hook_event = input_data.get("hook_event_name", "unknown")
    trigger = input_data.get("trigger", "auto")

    # Get project directory
    cwd = os.environ.get("CLAUDE_PROJECT_DIR", os.getcwd())

    # Check if we're in a wt worker session (has .wt directory or is a worktree)
    wt_dir = os.path.join(cwd, ".wt")
    is_wt_session = os.path.isdir(wt_dir) or is_git_worktree(cwd)

    if not is_wt_session:
        # Not a wt session, no need to checkpoint
        sys.exit(0)

    # Save checkpoint
    notes = f"PreCompact ({trigger})"
    try:
        result = subprocess.run(
            ["wt", "checkpoint", "-q", "-m", notes],
            cwd=cwd,
            capture_output=True,
            text=True,
            timeout=30
        )

        if result.returncode == 0:
            # Output context for Claude to see after compaction
            output = {
                "hookSpecificOutput": {
                    "hookEventName": "PreCompact",
                    "additionalContext": "Checkpoint saved. Run `wt prime` after compaction to recover context."
                }
            }
            print(json.dumps(output))
        else:
            # Checkpoint failed, but don't block compaction
            sys.stderr.write(f"wt checkpoint warning: {result.stderr}\n")

    except FileNotFoundError:
        # wt not installed, skip silently
        pass
    except subprocess.TimeoutExpired:
        sys.stderr.write("wt checkpoint timed out\n")
    except Exception as e:
        sys.stderr.write(f"wt checkpoint error: {e}\n")

    sys.exit(0)


def is_git_worktree(path):
    """Check if path is inside a git worktree"""
    try:
        result = subprocess.run(
            ["git", "rev-parse", "--git-common-dir"],
            cwd=path,
            capture_output=True,
            text=True,
            timeout=5
        )
        if result.returncode == 0:
            # If git-common-dir is different from .git, it's a worktree
            common_dir = result.stdout.strip()
            return not common_dir.endswith(".git")
    except Exception:
        pass
    return False


if __name__ == "__main__":
    main()
