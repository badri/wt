#!/usr/bin/env python3
"""
SessionStart hook to inject context on session startup.

This hook runs when a Claude session starts or resumes. It detects the
source (startup, resume, clear, compact) and runs `wt prime` to inject
appropriate context, including checkpoint recovery after compaction.

Add to your Claude settings:

{
  "hooks": {
    "SessionStart": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "/path/to/wt-session-start.py"
          }
        ]
      }
    ]
  }
}

Or use a simpler shell command:

{
  "hooks": {
    "SessionStart": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "wt prime --quiet 2>/dev/null || true"
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
        input_data = {}

    source = input_data.get("source", "startup")

    # Get project directory
    cwd = os.environ.get("CLAUDE_PROJECT_DIR", os.getcwd())

    # Check if we're in a wt worker session
    wt_dir = os.path.join(cwd, ".wt")
    is_wt_session = os.path.isdir(wt_dir) or is_git_worktree(cwd)

    if not is_wt_session:
        # Not a wt session, skip
        sys.exit(0)

    # Run wt prime to inject context
    try:
        result = subprocess.run(
            ["wt", "prime"],
            cwd=cwd,
            capture_output=True,
            text=True,
            timeout=30
        )

        if result.returncode == 0 and result.stdout.strip():
            # Output the prime result as context for Claude
            output = {
                "hookSpecificOutput": {
                    "hookEventName": "SessionStart",
                    "additionalContext": result.stdout
                }
            }
            print(json.dumps(output))
        elif result.returncode != 0:
            sys.stderr.write(f"wt prime warning: {result.stderr}\n")

    except FileNotFoundError:
        # wt not installed, skip silently
        pass
    except subprocess.TimeoutExpired:
        sys.stderr.write("wt prime timed out\n")
    except Exception as e:
        sys.stderr.write(f"wt prime error: {e}\n")

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
            common_dir = result.stdout.strip()
            return not common_dir.endswith(".git")
    except Exception:
        pass
    return False


if __name__ == "__main__":
    main()
