package scan

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// BenchmarkColdFixture establishes the Phase 0 baseline for a complete metadata walk.
func BenchmarkColdFixture(b *testing.B) {
	root := filepath.Join("..", "..", "testdata", "filesystems", "polyglot")
	for range b.N {
		entries := 0
		err := filepath.WalkDir(root, func(_ string, _ fs.DirEntry, err error) error {
			if err == nil {
				entries++
			}
			return err
		})
		if err != nil {
			b.Fatal(err)
		}
		if entries == 0 {
			b.Fatal("empty fixture")
		}
	}
}

// BenchmarkIncrementalFixture is a baseline for comparing cached identities.
func BenchmarkIncrementalFixture(b *testing.B) {
	root := filepath.Join("..", "..", "testdata", "filesystems", "polyglot")
	cache := make(map[string]int64)
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return infoErr
		}
		cache[path] = info.ModTime().UnixNano()
		return nil
	}); err != nil {
		b.Fatal(err)
	}
	if len(cache) < 8 {
		b.Fatalf("fixture unexpectedly small: %d entries", len(cache))
	}
	b.ResetTimer()
	for range b.N {
		for path, previous := range cache {
			info, statErr := os.Stat(path)
			if statErr != nil || info.ModTime().UnixNano() != previous {
				b.Fatal("fixture changed")
			}
		}
	}
}
