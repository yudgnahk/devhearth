package scan

import (
	"path/filepath"
	"sort"
	"strings"
)

// DirectoryNode is a rolled-up directory (or file) node for inventory drill-down.
// Totals include the node itself and all descendants under it.
type DirectoryNode struct {
	Path                string
	ParentPath          string
	Name                string
	Kind                string
	LogicalBytes        int64 // metadata size of this entry alone
	AllocatedBytes      int64 // allocated size of this entry alone
	TotalLogicalBytes   int64
	TotalAllocatedBytes int64
	DirectChildCount    int
	IsSymlink           bool
}

// BuildDirectoryIndex computes direct-child lists and directory rollups from a
// completed inventory. It does not touch the filesystem.
func BuildDirectoryIndex(result Result) map[string][]DirectoryNode {
	byPath := make(map[string]*DirectoryNode, len(result.Entries))
	children := make(map[string][]string, len(result.Entries)/2+1)

	for _, entry := range result.Entries {
		name := filepath.Base(entry.Path)
		if entry.Path == entry.ParentPath || entry.ParentPath == "" {
			// Roots still need a display name.
			if name == "" || name == string(filepath.Separator) {
				name = entry.Path
			}
		}
		node := &DirectoryNode{
			Path:                entry.Path,
			ParentPath:          entry.ParentPath,
			Name:                name,
			Kind:                entry.Kind,
			LogicalBytes:        entry.LogicalBytes,
			AllocatedBytes:      entry.AllocatedBytes,
			TotalLogicalBytes:   entry.LogicalBytes,
			TotalAllocatedBytes: entry.AllocatedBytes,
			IsSymlink:           entry.IsSymlink,
		}
		byPath[entry.Path] = node
		parent := entry.ParentPath
		children[parent] = append(children[parent], entry.Path)
	}

	// Deepest paths first so child totals exist before parents accumulate them.
	paths := make([]string, 0, len(byPath))
	for path := range byPath {
		paths = append(paths, path)
	}
	sort.Slice(paths, func(i, j int) bool {
		di, dj := strings.Count(paths[i], string(filepath.Separator)), strings.Count(paths[j], string(filepath.Separator))
		if di != dj {
			return di > dj
		}
		return paths[i] > paths[j]
	})

	for _, path := range paths {
		node := byPath[path]
		childPaths := children[path]
		node.DirectChildCount = len(childPaths)
		for _, childPath := range childPaths {
			child := byPath[childPath]
			node.TotalLogicalBytes += child.TotalLogicalBytes
			node.TotalAllocatedBytes += child.TotalAllocatedBytes
		}
	}

	// Parent maps to sorted direct children (not recursive).
	out := make(map[string][]DirectoryNode, len(children))
	for parent, childPaths := range children {
		sort.Strings(childPaths)
		nodes := make([]DirectoryNode, 0, len(childPaths))
		for _, childPath := range childPaths {
			nodes = append(nodes, *byPath[childPath])
		}
		out[parent] = nodes
	}
	return out
}

// ChildrenOf returns direct children for parentPath. Empty parentPath yields
// scan roots (entries whose ParentPath is empty).
func ChildrenOf(index map[string][]DirectoryNode, parentPath string) []DirectoryNode {
	if index == nil {
		return nil
	}
	return index[parentPath]
}
