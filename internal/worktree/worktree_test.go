package worktree

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSymlinkClaudeDir(t *testing.T) {
	t.Run("creates symlink when source exists", func(t *testing.T) {
		// Setup: create temp repo with .claude/ dir
		repoPath := t.TempDir()
		worktreePath := t.TempDir()
		srcDir := filepath.Join(repoPath, ".claude")
		dstDir := filepath.Join(worktreePath, ".claude")

		// Create source .claude/ with a file
		if err := os.MkdirAll(srcDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(srcDir, "settings.json"), []byte("{}"), 0644); err != nil {
			t.Fatal(err)
		}

		// Call function
		if err := SymlinkClaudeDir(repoPath, worktreePath); err != nil {
			t.Fatalf("SymlinkClaudeDir failed: %v", err)
		}

		// Verify symlink exists
		info, err := os.Lstat(dstDir)
		if err != nil {
			t.Fatalf("symlink not created: %v", err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			t.Error("expected symlink, got regular file/directory")
		}

		// Verify symlink target
		target, err := os.Readlink(dstDir)
		if err != nil {
			t.Fatalf("failed to read symlink: %v", err)
		}
		if target != srcDir {
			t.Errorf("symlink points to %s, expected %s", target, srcDir)
		}

		// Verify file is accessible through symlink
		content, err := os.ReadFile(filepath.Join(dstDir, "settings.json"))
		if err != nil {
			t.Fatalf("failed to read through symlink: %v", err)
		}
		if string(content) != "{}" {
			t.Errorf("unexpected content: %s", content)
		}
	})

	t.Run("no-op when source does not exist", func(t *testing.T) {
		repoPath := t.TempDir()
		worktreePath := t.TempDir()
		dstDir := filepath.Join(worktreePath, ".claude")

		// Call function (no .claude/ in repo)
		if err := SymlinkClaudeDir(repoPath, worktreePath); err != nil {
			t.Fatalf("SymlinkClaudeDir failed: %v", err)
		}

		// Verify no symlink created
		if _, err := os.Lstat(dstDir); !os.IsNotExist(err) {
			t.Error("symlink should not have been created")
		}
	})

	t.Run("no-op when destination already exists", func(t *testing.T) {
		repoPath := t.TempDir()
		worktreePath := t.TempDir()
		srcDir := filepath.Join(repoPath, ".claude")
		dstDir := filepath.Join(worktreePath, ".claude")

		// Create source
		if err := os.MkdirAll(srcDir, 0755); err != nil {
			t.Fatal(err)
		}

		// Create destination as a regular directory
		if err := os.MkdirAll(dstDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dstDir, "local.json"), []byte("local"), 0644); err != nil {
			t.Fatal(err)
		}

		// Call function
		if err := SymlinkClaudeDir(repoPath, worktreePath); err != nil {
			t.Fatalf("SymlinkClaudeDir failed: %v", err)
		}

		// Verify it's still a directory (not replaced by symlink)
		info, err := os.Lstat(dstDir)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			t.Error("existing directory was replaced by symlink")
		}

		// Verify local file still exists
		content, err := os.ReadFile(filepath.Join(dstDir, "local.json"))
		if err != nil {
			t.Fatal(err)
		}
		if string(content) != "local" {
			t.Error("local file was modified")
		}
	})

	t.Run("no-op when destination is already a symlink", func(t *testing.T) {
		repoPath := t.TempDir()
		worktreePath := t.TempDir()
		srcDir := filepath.Join(repoPath, ".claude")
		dstDir := filepath.Join(worktreePath, ".claude")

		// Create source
		if err := os.MkdirAll(srcDir, 0755); err != nil {
			t.Fatal(err)
		}

		// Create existing symlink to somewhere else
		otherTarget := t.TempDir()
		if err := os.Symlink(otherTarget, dstDir); err != nil {
			t.Fatal(err)
		}

		// Call function
		if err := SymlinkClaudeDir(repoPath, worktreePath); err != nil {
			t.Fatalf("SymlinkClaudeDir failed: %v", err)
		}

		// Verify symlink still points to original target
		target, err := os.Readlink(dstDir)
		if err != nil {
			t.Fatal(err)
		}
		if target != otherTarget {
			t.Errorf("symlink was modified: points to %s instead of %s", target, otherTarget)
		}
	})
}
