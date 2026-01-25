package doctor

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// initGitRepo initializes a git repository in the given path
func initGitRepo(t *testing.T, path string) {
	t.Helper()
	cmd := exec.Command("git", "init")
	cmd.Dir = path
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\nOutput: %s", err, out)
	}
}

func TestCheckClaudeMD(t *testing.T) {
	// Skip if git is not available
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	// Save and restore the original working directory
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origDir)

	t.Run("detects healthy project CLAUDE.md when present", func(t *testing.T) {
		// Create a temp git repo with CLAUDE.md
		repoPath := t.TempDir()

		// Initialize git repo properly
		initGitRepo(t, repoPath)

		// Create CLAUDE.md with enough content to be considered healthy (>= 50 bytes)
		claudeMD := filepath.Join(repoPath, "CLAUDE.md")
		content := "# Project Guidelines\n\nThis project follows best practices."
		if err := os.WriteFile(claudeMD, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		// Change to the repo directory
		if err := os.Chdir(repoPath); err != nil {
			t.Fatal(err)
		}

		results := checkClaudeMD()

		// Find the project CLAUDE.md result
		var found bool
		for _, r := range results {
			if r.Name == "project CLAUDE.md" {
				found = true
				if r.Status != "ok" {
					t.Errorf("expected status 'ok', got %q", r.Status)
				}
				if r.Message != "healthy" {
					t.Errorf("expected message 'healthy', got %q", r.Message)
				}
			}
		}
		if !found {
			t.Error("project CLAUDE.md check not found in results")
		}
	})

	t.Run("warns when project CLAUDE.md is missing", func(t *testing.T) {
		// Create a temp git repo without CLAUDE.md
		repoPath := t.TempDir()

		// Initialize git repo properly
		initGitRepo(t, repoPath)

		// Change to the repo directory
		if err := os.Chdir(repoPath); err != nil {
			t.Fatal(err)
		}

		results := checkClaudeMD()

		// Find the project CLAUDE.md result
		var found bool
		for _, r := range results {
			if r.Name == "project CLAUDE.md" {
				found = true
				if r.Status != "warn" {
					t.Errorf("expected status 'warn', got %q", r.Status)
				}
				if r.Message != "not found" {
					t.Errorf("expected message 'not found', got %q", r.Message)
				}
			}
		}
		if !found {
			t.Error("project CLAUDE.md check not found in results")
		}
	})

	t.Run("warns when project CLAUDE.md is empty", func(t *testing.T) {
		// Create a temp git repo with empty CLAUDE.md
		repoPath := t.TempDir()

		// Initialize git repo properly
		initGitRepo(t, repoPath)

		// Create empty CLAUDE.md
		claudeMD := filepath.Join(repoPath, "CLAUDE.md")
		if err := os.WriteFile(claudeMD, []byte(""), 0644); err != nil {
			t.Fatal(err)
		}

		// Change to the repo directory
		if err := os.Chdir(repoPath); err != nil {
			t.Fatal(err)
		}

		results := checkClaudeMD()

		// Find the project CLAUDE.md result
		var found bool
		for _, r := range results {
			if r.Name == "project CLAUDE.md" {
				found = true
				if r.Status != "warn" {
					t.Errorf("expected status 'warn', got %q", r.Status)
				}
				if r.Message != "empty file" {
					t.Errorf("expected message 'empty file', got %q", r.Message)
				}
			}
		}
		if !found {
			t.Error("project CLAUDE.md check not found in results")
		}
	})

	t.Run("warns when project CLAUDE.md has minimal content", func(t *testing.T) {
		// Create a temp git repo with minimal CLAUDE.md
		repoPath := t.TempDir()

		// Initialize git repo properly
		initGitRepo(t, repoPath)

		// Create CLAUDE.md with less than 50 bytes of content
		claudeMD := filepath.Join(repoPath, "CLAUDE.md")
		if err := os.WriteFile(claudeMD, []byte("# Project"), 0644); err != nil {
			t.Fatal(err)
		}

		// Change to the repo directory
		if err := os.Chdir(repoPath); err != nil {
			t.Fatal(err)
		}

		results := checkClaudeMD()

		// Find the project CLAUDE.md result
		var found bool
		for _, r := range results {
			if r.Name == "project CLAUDE.md" {
				found = true
				if r.Status != "warn" {
					t.Errorf("expected status 'warn', got %q", r.Status)
				}
				if r.Message != "minimal content" {
					t.Errorf("expected message 'minimal content', got %q", r.Message)
				}
			}
		}
		if !found {
			t.Error("project CLAUDE.md check not found in results")
		}
	})

	t.Run("detects .claude directory as symlink", func(t *testing.T) {
		// Create a temp git repo with .claude as symlink
		repoPath := t.TempDir()
		targetDir := t.TempDir()

		// Initialize git repo properly
		initGitRepo(t, repoPath)

		// Create .claude as symlink
		claudeDir := filepath.Join(repoPath, ".claude")
		if err := os.Symlink(targetDir, claudeDir); err != nil {
			t.Fatal(err)
		}

		// Create CLAUDE.md with enough content to be healthy (>= 50 bytes)
		claudeMD := filepath.Join(repoPath, "CLAUDE.md")
		content := "# Project Guidelines\n\nThis project follows best practices."
		if err := os.WriteFile(claudeMD, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		// Change to the repo directory
		if err := os.Chdir(repoPath); err != nil {
			t.Fatal(err)
		}

		results := checkClaudeMD()

		// Find the project .claude/ result
		var found bool
		for _, r := range results {
			if r.Name == "project .claude/" {
				found = true
				if r.Status != "ok" {
					t.Errorf("expected status 'ok', got %q", r.Status)
				}
				if r.Message == "" {
					t.Error("expected message to contain symlink info")
				}
			}
		}
		if !found {
			t.Error("project .claude/ check not found in results")
		}
	})

	t.Run("skips project checks when not in git repo", func(t *testing.T) {
		// Create a temp directory that's not a git repo
		nonRepoPath := t.TempDir()

		// Change to the non-repo directory
		if err := os.Chdir(nonRepoPath); err != nil {
			t.Fatal(err)
		}

		results := checkClaudeMD()

		// Should only have global CLAUDE.md check, no project checks
		for _, r := range results {
			if r.Name == "project CLAUDE.md" || r.Name == "project .claude/" {
				t.Errorf("unexpected project check %q when not in git repo", r.Name)
			}
		}
	})
}

func TestGetGitRoot(t *testing.T) {
	// Save and restore the original working directory
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origDir)

	t.Run("returns empty string when not in git repo", func(t *testing.T) {
		tempDir := t.TempDir()
		if err := os.Chdir(tempDir); err != nil {
			t.Fatal(err)
		}

		root := getGitRoot()
		if root != "" {
			t.Errorf("expected empty string, got %q", root)
		}
	})

	t.Run("returns root when in git repo", func(t *testing.T) {
		// We're running in a real git repo, so this should work
		if err := os.Chdir(origDir); err != nil {
			t.Fatal(err)
		}

		root := getGitRoot()
		if root == "" {
			t.Skip("not running in a git repo")
		}
		// Just verify it returns something reasonable
		if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
			t.Errorf("returned path %q doesn't contain .git", root)
		}
	})
}

func TestClaudeMDHealthCheck(t *testing.T) {
	t.Run("returns healthy for valid content", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "CLAUDE.md")
		content := "# Project Guidelines\n\nThis project follows coding best practices and standards."
		if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}

		status, message, _ := claudeMDHealthCheck(tmpFile)
		if status != "ok" {
			t.Errorf("expected status 'ok', got %q", status)
		}
		if message != "healthy" {
			t.Errorf("expected message 'healthy', got %q", message)
		}
	})

	t.Run("warns for empty file", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "CLAUDE.md")
		if err := os.WriteFile(tmpFile, []byte(""), 0644); err != nil {
			t.Fatal(err)
		}

		status, message, _ := claudeMDHealthCheck(tmpFile)
		if status != "warn" {
			t.Errorf("expected status 'warn', got %q", status)
		}
		if message != "empty file" {
			t.Errorf("expected message 'empty file', got %q", message)
		}
	})

	t.Run("warns for whitespace-only file", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "CLAUDE.md")
		if err := os.WriteFile(tmpFile, []byte("   \n\n\t  "), 0644); err != nil {
			t.Fatal(err)
		}

		status, message, _ := claudeMDHealthCheck(tmpFile)
		if status != "warn" {
			t.Errorf("expected status 'warn', got %q", status)
		}
		if message != "empty file" {
			t.Errorf("expected message 'empty file', got %q", message)
		}
	})

	t.Run("warns for minimal content", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "CLAUDE.md")
		// Less than 50 bytes of actual content
		if err := os.WriteFile(tmpFile, []byte("# Project"), 0644); err != nil {
			t.Fatal(err)
		}

		status, message, _ := claudeMDHealthCheck(tmpFile)
		if status != "warn" {
			t.Errorf("expected status 'warn', got %q", status)
		}
		if message != "minimal content" {
			t.Errorf("expected message 'minimal content', got %q", message)
		}
	})

	t.Run("warns for large file", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "CLAUDE.md")
		// Create content larger than 50KB
		content := make([]byte, 60*1024)
		for i := range content {
			content[i] = 'a'
		}
		if err := os.WriteFile(tmpFile, content, 0644); err != nil {
			t.Fatal(err)
		}

		status, message, _ := claudeMDHealthCheck(tmpFile)
		if status != "warn" {
			t.Errorf("expected status 'warn', got %q", status)
		}
		if message != "large file (60 KB)" {
			t.Errorf("expected message 'large file (60 KB)', got %q", message)
		}
	})

	t.Run("warns for unreadable file", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "nonexistent", "CLAUDE.md")

		status, message, _ := claudeMDHealthCheck(tmpFile)
		if status != "warn" {
			t.Errorf("expected status 'warn', got %q", status)
		}
		if message != "cannot read" {
			t.Errorf("expected message 'cannot read', got %q", message)
		}
	})
}
