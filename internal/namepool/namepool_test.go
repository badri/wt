package namepool

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/badri/wt/internal/config"
)

func TestLoad(t *testing.T) {
	tmpDir := t.TempDir()

	// Create namepool file
	namepoolContent := "alpha\nbeta\ngamma\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "namepool.txt"), []byte(namepoolContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create sessions.json (required by config)
	if err := os.WriteFile(filepath.Join(tmpDir, "sessions.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.LoadFromDir(tmpDir)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	pool, err := Load(cfg)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	names := pool.Names()
	if len(names) != 3 {
		t.Errorf("expected 3 names, got %d", len(names))
	}

	expected := []string{"alpha", "beta", "gamma"}
	for i, name := range expected {
		if names[i] != name {
			t.Errorf("expected name[%d] = %q, got %q", i, name, names[i])
		}
	}
}

func TestLoad_SkipsEmptyLines(t *testing.T) {
	tmpDir := t.TempDir()

	// Create namepool with empty lines
	namepoolContent := "alpha\n\nbeta\n  \ngamma\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "namepool.txt"), []byte(namepoolContent), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "sessions.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, _ := config.LoadFromDir(tmpDir)
	pool, err := Load(cfg)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(pool.Names()) != 3 {
		t.Errorf("expected 3 names after skipping empty lines, got %d", len(pool.Names()))
	}
}

func TestAllocate_ReturnsFirstAvailable(t *testing.T) {
	pool := NewPool([]string{"alpha", "beta", "gamma"})

	name, err := pool.Allocate(nil)
	if err != nil {
		t.Fatalf("Allocate failed: %v", err)
	}
	if name != "alpha" {
		t.Errorf("expected 'alpha', got %q", name)
	}
}

func TestAllocate_SkipsUsedNames(t *testing.T) {
	pool := NewPool([]string{"alpha", "beta", "gamma"})

	name, err := pool.Allocate([]string{"alpha"})
	if err != nil {
		t.Fatalf("Allocate failed: %v", err)
	}
	if name != "beta" {
		t.Errorf("expected 'beta', got %q", name)
	}
}

func TestAllocate_SkipsMultipleUsedNames(t *testing.T) {
	pool := NewPool([]string{"alpha", "beta", "gamma", "delta"})

	name, err := pool.Allocate([]string{"alpha", "beta"})
	if err != nil {
		t.Fatalf("Allocate failed: %v", err)
	}
	if name != "gamma" {
		t.Errorf("expected 'gamma', got %q", name)
	}
}

func TestAllocate_ReturnsErrorWhenExhausted(t *testing.T) {
	pool := NewPool([]string{"alpha", "beta"})

	_, err := pool.Allocate([]string{"alpha", "beta"})
	if err == nil {
		t.Error("expected error when pool exhausted, got nil")
	}
}

func TestAllocate_EmptyPool(t *testing.T) {
	pool := NewPool([]string{})

	_, err := pool.Allocate(nil)
	if err == nil {
		t.Error("expected error for empty pool, got nil")
	}
}

func TestNames(t *testing.T) {
	expected := []string{"one", "two", "three"}
	pool := NewPool(expected)

	names := pool.Names()
	if len(names) != len(expected) {
		t.Errorf("expected %d names, got %d", len(expected), len(names))
	}

	for i, name := range expected {
		if names[i] != name {
			t.Errorf("expected names[%d] = %q, got %q", i, name, names[i])
		}
	}
}
