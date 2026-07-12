// Package scan performs metadata-only filesystem inventory. It never reads
// file contents or follows symbolic links.
package scan

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

type Entry struct {
	Path           string    `json:"path"`
	ParentPath     string    `json:"parentPath,omitempty"`
	Kind           string    `json:"kind"`
	LogicalBytes   int64     `json:"logicalBytes"`
	AllocatedBytes int64     `json:"allocatedBytes"`
	DeviceID       uint64    `json:"deviceId"`
	Inode          uint64    `json:"inode"`
	LinkCount      uint64    `json:"linkCount"`
	IsSymlink      bool      `json:"isSymlink"`
	HardLinkAlias  bool      `json:"hardLinkAlias"`
	ModifiedAt     time.Time `json:"modifiedAt"`
}

type InaccessiblePath struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

type Result struct {
	Roots          []string           `json:"roots"`
	Entries        []Entry            `json:"entries"`
	Inaccessible   []InaccessiblePath `json:"inaccessible"`
	EntriesVisited int64              `json:"entriesVisited"`
	LogicalBytes   int64              `json:"logicalBytes"`
	AllocatedBytes int64              `json:"allocatedBytes"`
	StartedAt      time.Time          `json:"startedAt"`
	CompletedAt    time.Time          `json:"completedAt"`
}

type Progress struct {
	Phase          string
	EntriesVisited int64
	AllocatedBytes int64
}

type Options struct {
	// ProgressInterval controls how often metadata progress is emitted. Zero
	// selects a conservative default. Traversal intentionally uses one worker:
	// it is bounded, cancellation-friendly, and avoids unbounded directory work.
	ProgressInterval int
	Progress         func(Progress)
}

func Inventory(ctx context.Context, roots []string, options Options) (Result, error) {
	if len(roots) == 0 {
		return Result{}, errors.New("at least one scan root is required")
	}
	if options.ProgressInterval <= 0 {
		options.ProgressInterval = 256
	}
	result := Result{StartedAt: time.Now().UTC()}
	normalizedRoots, err := normalizeRoots(roots)
	if err != nil {
		return result, err
	}
	seenInodes := make(map[fileIdentity]struct{})
	for _, root := range normalizedRoots {
		rootInfo, err := os.Lstat(root)
		if err != nil {
			result.Inaccessible = append(result.Inaccessible, InaccessiblePath{Path: root, Reason: err.Error()})
			continue
		}
		rootID, ok := identity(rootInfo)
		if !ok {
			return result, fmt.Errorf("read filesystem identity for %q", root)
		}
		result.Roots = append(result.Roots, root)
		if err := walkRoot(ctx, root, rootID.device, seenInodes, options, &result); err != nil {
			if errors.Is(err, context.Canceled) {
				return result, err
			}
			return result, fmt.Errorf("scan %q: %w", root, err)
		}
	}
	if len(result.Roots) == 0 {
		return result, errors.New("none of the requested scan roots are accessible")
	}
	sort.Slice(result.Entries, func(i, j int) bool { return result.Entries[i].Path < result.Entries[j].Path })
	result.CompletedAt = time.Now().UTC()
	return result, nil
}

func normalizeRoots(inputs []string) ([]string, error) {
	seen := make(map[string]struct{}, len(inputs))
	roots := make([]string, 0, len(inputs))
	for _, input := range inputs {
		root, err := filepath.Abs(filepath.Clean(input))
		if err != nil {
			return nil, fmt.Errorf("resolve scan root %q: %w", input, err)
		}
		if _, exists := seen[root]; !exists {
			seen[root] = struct{}{}
			roots = append(roots, root)
		}
	}
	sort.Slice(roots, func(i, j int) bool {
		if len(roots[i]) == len(roots[j]) {
			return roots[i] < roots[j]
		}
		return len(roots[i]) < len(roots[j])
	})
	normalized := make([]string, 0, len(roots))
	for _, root := range roots {
		covered := false
		for _, selected := range normalized {
			if isWithinRoot(root, selected) {
				covered = true
				break
			}
		}
		if !covered {
			normalized = append(normalized, root)
		}
	}
	return normalized, nil
}

func isWithinRoot(path, root string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

type fileIdentity struct{ device, inode uint64 }

func walkRoot(ctx context.Context, root string, device uint64, seenInodes map[fileIdentity]struct{}, options Options, result *Result) error {
	var walk func(string, string) error
	walk = func(path, parent string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			result.Inaccessible = append(result.Inaccessible, InaccessiblePath{Path: path, Reason: err.Error()})
			return nil
		}
		id, ok := identity(info)
		if !ok {
			return fmt.Errorf("read filesystem identity for %q", path)
		}
		if id.device != device {
			result.Inaccessible = append(result.Inaccessible, InaccessiblePath{Path: path, Reason: "different filesystem volume"})
			return nil
		}
		entry := makeEntry(path, parent, info, id)
		if !entry.IsSymlink && info.Mode().IsRegular() && entry.LinkCount > 1 {
			if _, exists := seenInodes[id]; exists {
				entry.HardLinkAlias = true
				entry.AllocatedBytes = 0
			}
			seenInodes[id] = struct{}{}
		}
		result.Entries = append(result.Entries, entry)
		result.EntriesVisited++
		result.LogicalBytes += entry.LogicalBytes
		result.AllocatedBytes += entry.AllocatedBytes
		if options.Progress != nil && result.EntriesVisited%int64(options.ProgressInterval) == 0 {
			options.Progress(Progress{Phase: "metadata", EntriesVisited: result.EntriesVisited, AllocatedBytes: result.AllocatedBytes})
		}
		if !info.IsDir() || entry.IsSymlink {
			return nil
		}
		children, err := os.ReadDir(path)
		if err != nil {
			result.Inaccessible = append(result.Inaccessible, InaccessiblePath{Path: path, Reason: err.Error()})
			return nil
		}
		for _, child := range children {
			if err := walk(filepath.Join(path, child.Name()), path); err != nil {
				return err
			}
		}
		return nil
	}
	return walk(root, "")
}

func makeEntry(path, parent string, info fs.FileInfo, id fileIdentity) Entry {
	kind := "other"
	if info.Mode()&fs.ModeSymlink != 0 {
		kind = "symlink"
	} else if info.IsDir() {
		kind = "directory"
	} else if info.Mode().IsRegular() {
		kind = "file"
	}
	blocks := allocatedBlocks(info)
	return Entry{Path: path, ParentPath: parent, Kind: kind, LogicalBytes: info.Size(), AllocatedBytes: blocks * 512, DeviceID: id.device, Inode: id.inode, LinkCount: linkCount(info), IsSymlink: kind == "symlink", ModifiedAt: info.ModTime().UTC()}
}

func identity(info fs.FileInfo) (fileIdentity, bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fileIdentity{}, false
	}
	return fileIdentity{device: uint64(stat.Dev), inode: uint64(stat.Ino)}, true
}

func allocatedBlocks(info fs.FileInfo) int64 {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return stat.Blocks
	}
	return 0
}

func linkCount(info fs.FileInfo) uint64 {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return uint64(stat.Nlink)
	}
	return 1
}
