package scan_test

import (
	"testing"

	"github.com/yudgnahk/devhearth/internal/scan"
)

func TestBuildDirectoryIndexRollsUpTotals(t *testing.T) {
	result := scan.Result{
		Roots: []string{"/r"},
		Entries: []scan.Entry{
			{Path: "/r", Kind: "directory"},
			{Path: "/r/a", ParentPath: "/r", Kind: "directory"},
			{Path: "/r/a/f", ParentPath: "/r/a", Kind: "file", LogicalBytes: 10, AllocatedBytes: 4096},
			{Path: "/r/b", ParentPath: "/r", Kind: "file", LogicalBytes: 3, AllocatedBytes: 4096},
		},
	}
	index := scan.BuildDirectoryIndex(result)
	roots := scan.ChildrenOf(index, "")
	if len(roots) != 1 || roots[0].Path != "/r" {
		t.Fatalf("roots = %#v", roots)
	}
	children := scan.ChildrenOf(index, "/r")
	if len(children) != 2 {
		t.Fatalf("children of /r = %#v", children)
	}
	var dirA scan.DirectoryNode
	for _, child := range children {
		if child.Path == "/r/a" {
			dirA = child
		}
	}
	if dirA.TotalLogicalBytes != 10 || dirA.TotalAllocatedBytes != 4096 {
		t.Fatalf("dir A totals = %#v", dirA)
	}
	// Root totals include a (10/4096) + b (3/4096) + own 0.
	if roots[0].TotalLogicalBytes != 13 || roots[0].TotalAllocatedBytes != 8192 {
		t.Fatalf("root totals = %#v", roots[0])
	}
	if dirA.DirectChildCount != 1 {
		t.Fatalf("dir A child count = %d", dirA.DirectChildCount)
	}
}
