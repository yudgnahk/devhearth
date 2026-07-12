package scan

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestInventoryDoesNotFollowSymlinksAndDeduplicatesHardLinks(t *testing.T) {
	root := t.TempDir()
	data := filepath.Join(root, "data")
	if err := os.WriteFile(data, []byte("inventory"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(data, filepath.Join(root, "data-link")); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "outside-link")); err != nil {
		t.Fatal(err)
	}

	result, err := Inventory(context.Background(), []string{root}, Options{ProgressInterval: 1})
	if err != nil {
		t.Fatal(err)
	}
	var aliases, symlinks int
	for _, entry := range result.Entries {
		if entry.HardLinkAlias {
			aliases++
		}
		if entry.IsSymlink {
			symlinks++
		}
	}
	if aliases != 1 {
		t.Fatalf("hard link aliases = %d, want 1", aliases)
	}
	if symlinks != 1 {
		t.Fatalf("symlinks = %d, want 1", symlinks)
	}
	for _, entry := range result.Entries {
		if entry.Path == outside {
			t.Fatalf("followed symlink to %q", outside)
		}
	}
}

func TestInventoryHonorsCancellation(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 10; i++ {
		if err := os.WriteFile(filepath.Join(root, string(rune('a'+i))), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	_, err := Inventory(ctx, []string{root}, Options{ProgressInterval: 1, Progress: func(Progress) { cancel() }})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want cancellation", err)
	}
}

func TestInventoryDoesNotDuplicateNestedRoots(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "child")
	if err := os.Mkdir(child, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(child, "data"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := Inventory(context.Background(), []string{root, child}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Roots) != 1 || result.Roots[0] != root {
		t.Fatalf("roots = %#v, want only %q", result.Roots, root)
	}
	if result.EntriesVisited != 3 {
		t.Fatalf("entries = %d, want 3", result.EntriesVisited)
	}
}
