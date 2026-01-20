#!/usr/bin/env python3
"""
PreToolUse hook to format Go code before git commits.
Runs 'go fmt ./...' and 'go vet ./...' before allowing git commit commands.
"""
import json
import sys
import subprocess
import re
import os

def main():
    # Load hook input from stdin
    try:
        input_data = json.load(sys.stdin)
    except json.JSONDecodeError:
        sys.exit(0)  # Invalid input, allow through

    tool_name = input_data.get("tool_name", "")
    tool_input = input_data.get("tool_input", {})
    command = tool_input.get("command", "")

    # Only intercept git commit commands (not push - formatting should happen before commit)
    if tool_name != "Bash" or not re.search(r"git\s+commit", command):
        sys.exit(0)

    # Get project directory
    cwd = os.environ.get("CLAUDE_PROJECT_DIR", os.getcwd())

    messages = []

    # Run go fmt
    try:
        result = subprocess.run(
            ["go", "fmt", "./..."],
            cwd=cwd,
            capture_output=True,
            text=True,
            timeout=30
        )
        if result.stdout.strip():
            messages.append(f"Formatted: {result.stdout.strip()}")
    except Exception as e:
        messages.append(f"go fmt warning: {e}")

    # Run go vet
    try:
        result = subprocess.run(
            ["go", "vet", "./..."],
            cwd=cwd,
            capture_output=True,
            text=True,
            timeout=30
        )
        if result.returncode != 0:
            # Vet found issues - block the commit
            output = {
                "hookSpecificOutput": {
                    "hookEventName": "PreToolUse",
                    "permissionDecision": "deny",
                    "permissionDecisionReason": f"go vet found issues:\n{result.stderr or result.stdout}\n\nFix these before committing."
                }
            }
            print(json.dumps(output))
            sys.exit(0)
    except Exception as e:
        messages.append(f"go vet warning: {e}")

    # Allow commit with info about what was done
    if messages:
        output = {
            "hookSpecificOutput": {
                "hookEventName": "PreToolUse",
                "permissionDecision": "allow",
                "permissionDecisionReason": "Pre-commit checks passed",
                "additionalContext": "\n".join(messages)
            }
        }
        print(json.dumps(output))

    sys.exit(0)

if __name__ == "__main__":
    main()
